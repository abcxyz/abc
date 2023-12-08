package describe

import (
	"fmt"
	"strings"

	"github.com/abcxyz/pkg/cli"
	"github.com/posener/complete/v2/predict"
)

// DescribeFlags describes what template to describe.
type DescribeFlags struct {
	// Positional arguments:

	// Source is the location of the input template to be rendered.
	//
	// Example: github.com/abcxyz/abc/t/rest_server@latest
	Source string

	// GitProtocol is not yet used.
	GitProtocol string
}

func (r *DescribeFlags) Register(set *cli.FlagSet) {

	g := set.NewSection("GIT OPTIONS")
	g.StringVar(&cli.StringVar{
		Name:    "git-protocol",
		Example: "https",
		Default: "https",
		Target:  &r.GitProtocol,
		Predict: predict.Set([]string{"https", "ssh"}),
		Usage:   "Either ssh or https, the protocol for connecting to git. Only used if the template source is a git repo.",
	})

	// Default source to the first CLI argument, if given
	set.AfterParse(func(existingErr error) error {
		r.Source = strings.TrimSpace(set.Arg(0))
		if r.Source == "" {
			return fmt.Errorf("missing <source> file")
		}

		return nil
	})
}
