// Copyright 2023 The Authors (see AUTHORS file)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package input

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/mattn/go-isatty"
	"golang.org/x/exp/maps"
	"gopkg.in/yaml.v3"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/rules"
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta6"
	"github.com/abcxyz/pkg/sets"
)

// ResolveParams are the parameters to Resolve(), wrapped in a struct because
// there are so many.
type ResolveParams struct {
	FS common.FS

	// Whether to silently accept default values for template inputs in the case
	// where prompting is disabled.
	AcceptDefaults bool

	// Ignore any values in the Inputs map that aren't valid template inputs,
	// rather than returning error.
	IgnoreUnknownInputs bool

	// The value of --input. Template input values.
	Inputs map[string]string

	// This is only set in the case where this template is being rendered as
	// part of an upgrade operation, and contains the set of inputs that were
	// saved in the manifest from the previous render operation. They're
	// separate from the other inputs so they can be given lowest precedence.
	InputsFromManifest map[string]string

	// The value of --input-file. A list of YAML filenames defining template inputs.
	InputFiles []string

	// Prompt is the value of --prompt, it enables or disables the prompting feature.
	Prompt bool

	// Prompter is used to print prompts to the user requesting them to enter
	// input.
	Prompter            Prompter
	SkipInputValidation bool

	// Normally, we'll only prompt if the input is a TTY. For testing, this
	// can be set to true to bypass the check and allow stdin to be something
	// other than a TTY, like an os.Pipe.
	SkipPromptTTYCheck bool

	// The template spec.yaml model.
	Spec *spec.Spec
}

// Prompter prints messages to the user asking them to enter a value. This is
// implemented by *cli.Command.
type Prompter interface {
	Prompt(ctx context.Context, msg string, args ...any) (string, error)
	Stdin() io.Reader
}

// Resolve combines flags, user prompts, and defaults to get the full set
// of template inputs.
func Resolve(ctx context.Context, rp *ResolveParams) (map[string]string, error) {
	if badInputs := checkReservedInputs(rp.Inputs); len(badInputs) > 0 {
		return nil, fmt.Errorf(`input names beginning with underscore cannot be overridden by a normal user input; the bad input names were: %v`, badInputs)
	}

	if !rp.IgnoreUnknownInputs {
		if unknownInputs := checkUnknownInputs(rp.Spec, rp.Inputs); len(unknownInputs) > 0 {
			return nil, fmt.Errorf("unknown input(s): %s", strings.Join(unknownInputs, ", "))
		}
	}
	cliInputs := filterUnknownInputs(rp.Spec, rp.Inputs)

	fileInputs, err := loadInputFiles(rp.FS, rp.InputFiles)
	if err != nil {
		return nil, err
	}

	// Unknown inputs from --input-file files are always ignored regardless of
	// the --ignore-unknown-inputs flag.
	knownFileInputs := filterUnknownInputs(rp.Spec, fileInputs)

	// Unknown inputs from manifest files are always ignored regardless of
	// the --ignore-unknown-inputs flag, because a new template version may
	// remove an input that was present in an old version, we don't want to
	// include such superfluous inputs in the render process.
	knownInputsFromManifest := filterUnknownInputs(rp.Spec, rp.InputsFromManifest)

	// Order matters: values from --input take precedence over --input-file
	// which in turn take precedence over manifest inputs.
	inputs := sets.UnionMapKeys(cliInputs, knownFileInputs, knownInputsFromManifest)

	if rp.Prompt {
		_, ok := rp.Prompter.(fakePrompter)
		runningUnderTest := ok || rp.SkipPromptTTYCheck

		// When running in a test, we allow prompting, even though stdin has
		// been faked for testing and isn't a terminal. This lets us test the
		// prompting logic.
		if !runningUnderTest {
			isATTY := (rp.Prompter.Stdin() == os.Stdin && isatty.IsTerminal(os.Stdin.Fd()))
			if !isATTY {
				return nil, fmt.Errorf("the flag --prompt was provided, but standard input is not a terminal")
			}
		}

		if err := promptForInputs(ctx, rp.Prompter, rp.Spec, inputs); err != nil {
			return nil, err
		}
	} else {
		defaulted := insertDefaultInputs(rp.Spec, inputs)
		if missing := checkInputsMissing(rp.Spec, inputs); len(missing) > 0 {
			return nil, fmt.Errorf("missing input(s): %s, you may want to use one of the flags --prompt, --input, or --input-file", strings.Join(missing, ", "))
		}
		if len(defaulted) > 0 && !rp.AcceptDefaults {
			// This avoids a specific poor user experience. Suppose the user
			// runs `abc upgrade` (without --prompt), which is a very reasonable
			// thing to do. Suppose the upgraded version of the template has a
			// new input with a default value for that input. It would be bad
			// for abc to just silently use the default value for that new
			// input; technically you could argue that the user should have used
			// the --prompt flag if they wanted a chance to override the
			// default, but that would be a footgun and we can't expect users to
			// be that diligent. So we'll reject the current operation and ask
			// the user to clarify their intent with either --prompt or
			// --accept-defaults.
			return nil, fmt.Errorf("there are some inputs for which a value was not provided but a default is available; please use either --prompt or --accept-defaults: %v", defaulted)
		}
	}

	if rp.SkipInputValidation {
		return inputs, nil
	}

	if err := validateInputs(ctx, rp.Spec.Inputs, inputs); err != nil {
		return nil, err
	}

	return inputs, nil
}

// This interface is satisfied by *prompt.FakePrompter.
type fakePrompter interface {
	IsTestFake()
}

func validateInputs(ctx context.Context, specInputs []*spec.Input, inputVals map[string]string) error {
	scope := common.NewScope(inputVals, nil)

	sb := &strings.Builder{}
	tw := tabwriter.NewWriter(sb, 8, 0, 2, ' ', 0)

	for _, input := range specInputs {
		rules.ValidateRulesWithMessage(ctx, scope, input.Rules, tw, func() {
			fmt.Fprintf(tw, "\nInput name:\t%s", input.Name.Val)
			fmt.Fprintf(tw, "\nInput value:\t%s", inputVals[input.Name.Val])
		})
	}

	tw.Flush()
	if sb.Len() > 0 {
		return fmt.Errorf("input validation failed:\n%s", sb.String())
	}
	return nil
}

// promptForInputs looks for template inputs that were not provided on the
// command line and prompts the user for them. This mutates "inputs".
//
// This must only be called when the user specified --prompt and the input is a
// terminal (or in a test).
func promptForInputs(ctx context.Context, prompter Prompter, spec *spec.Spec, inputs map[string]string) error {
	for _, i := range spec.Inputs {
		if _, ok := inputs[i.Name.Val]; ok {
			// Don't prompt if we already have a value for this input.
			continue
		}
		sb := &strings.Builder{}
		tw := tabwriter.NewWriter(sb, 8, 0, 2, ' ', 0)
		fmt.Fprintf(tw, "\nInput name:\t%s", i.Name.Val)
		fmt.Fprintf(tw, "\nDescription:\t%s", i.Desc.Val)
		for idx, rule := range i.Rules {
			printRuleIndex := len(i.Rules) > 1
			rules.WriteRule(tw, rule, printRuleIndex, idx)
		}

		if i.Default != nil {
			defaultStr := i.Default.Val
			if defaultStr == "" {
				// When empty string is the default, print it differently so
				// the user can actually see what's happening.
				defaultStr = `""`
			}
			fmt.Fprintf(tw, "\nDefault:\t%s", defaultStr)
		}

		tw.Flush()

		if i.Default != nil {
			fmt.Fprintf(sb, "\n\nEnter value, or leave empty to accept default: ")
		} else {
			fmt.Fprintf(sb, "\n\nEnter value: ")
		}

		inputVal, err := prompter.Prompt(ctx, sb.String())
		if err != nil {
			return fmt.Errorf("failed to prompt for user input: %w", err)
		}

		if inputVal == "" && i.Default != nil {
			inputVal = i.Default.Val
		}

		inputs[i.Name.Val] = inputVal
	}
	return nil
}

func checkReservedInputs(inputs map[string]string) []string {
	var bad []string
	for input := range inputs {
		if strings.HasPrefix(input, "_") {
			bad = append(bad, input)
		}
	}
	sort.Strings(bad)
	return bad
}

// checkUnknownInputs checks for any unknown input flags and returns them in a slice.
func checkUnknownInputs(spec *spec.Spec, inputs map[string]string) []string {
	specInputs := make([]string, 0, len(spec.Inputs))
	for _, v := range spec.Inputs {
		specInputs = append(specInputs, v.Name.Val)
	}

	seenInputs := maps.Keys(inputs)
	unknownInputs := sets.Subtract(seenInputs, specInputs)
	sort.Strings(unknownInputs)
	return unknownInputs
}

// filterUnknownInputs returns the subset of the given inputs that appear in the
// given spec. Essentially it discards any inputs that aren't declared in the
// spec.
func filterUnknownInputs(spec *spec.Spec, inputs map[string]string) map[string]string {
	specInputs := make(map[string]struct{})

	for _, v := range spec.Inputs {
		specInputs[v.Name.Val] = struct{}{}
	}

	out := make(map[string]string, len(specInputs))
	for k, v := range inputs {
		if _, ok := specInputs[k]; ok {
			out[k] = v
		}
	}

	return out
}

// loadInputFiles iterates over each --input-file and combines them all into a map.
func loadInputFiles(fs common.FS, paths []string) (map[string]string, error) {
	out := make(map[string]string)
	sourceFileForInput := make(map[string]string)

	for _, f := range paths {
		inputsThisFile, err := loadInputFile(fs, f)
		if err != nil {
			return nil, err
		}

		for key, val := range inputsThisFile {
			if _, ok := out[key]; ok {
				return nil, fmt.Errorf("input key %q appears in multiple input files %q and %q; there must not be any overlap between input files",
					key, f, sourceFileForInput[key])
			}

			out[key] = val
			sourceFileForInput[key] = f
		}
	}
	return out, nil
}

// insertDefaultInputs defaults any missing inputs for which a default exists.
// The input map will be mutated by adding new keys. The return value is the
// list of input names that had default values set because they were not already
// set.
func insertDefaultInputs(spec *spec.Spec, userInputs map[string]string) []string {
	var defaulted []string //nolint:prealloc

	for _, specInput := range spec.Inputs {
		_, userGaveInput := userInputs[specInput.Name.Val]
		defaultExists := specInput.Default != nil
		if userGaveInput || !defaultExists {
			continue
		}

		userInputs[specInput.Name.Val] = specInput.Default.Val
		defaulted = append(defaulted, specInput.Name.Val)
	}

	return defaulted
}

// checkInputsMissing checks for missing inputs and returns them as a slice.
func checkInputsMissing(spec *spec.Spec, inputs map[string]string) []string {
	missing := make([]string, 0, len(inputs))

	for _, input := range spec.Inputs {
		if _, ok := inputs[input.Name.Val]; !ok {
			missing = append(missing, input.Name.Val)
		}
	}

	sort.Strings(missing)

	return missing
}

// loadInputFile loads a single --input-file into a map.
func loadInputFile(fs common.FS, path string) (map[string]string, error) {
	data, err := fs.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading input file: %w", err)
	}
	m := make(map[string]string)
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("error parsing yaml file: %w", err)
	}
	return m, nil
}
