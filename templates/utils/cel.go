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

package utils

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/pkg/logging"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

var celRegistry = types.NewEmptyRegistry()

// Any number less than this is assumed NOT to be a valid GCP project ID.
const minProjectNum = 1000

// Regex definitions for GCP entities.
// See https://github.com/hashicorp/terraform-provider-google/blob/a9cfeea162e19012fb662f8f0d89339daebf61a2/google/verify/validation.go#L20
var (
	gcpProjectIDRegexPart = `(?:(?:[-a-z0-9]{1,63}\.)*(?:[a-z](?:[-a-z0-9]{0,61}[a-z0-9])?):)?(?:[0-9]{1,19}|(?:[a-z0-9](?:[-a-z0-9]{0,61}[a-z0-9])?))`
	gcpProjectIDRegex     = regexp.MustCompile(`^` + gcpProjectIDRegexPart + `$`)

	// A service account "ID" is the part before the "@".
	gcpSvcAcctID      = `[a-z](?:[-a-z0-9]{4,28}[a-z0-9])`
	gcpSvcAcctIDRegex = regexp.MustCompile(`^` + gcpSvcAcctID + `$`)

	// A regex for a service account created by a user using the API (not an agent).
	gcpCreatedSvcAcctRegex = regexp.MustCompile(`^` + gcpSvcAcctID + `@[-a-z0-9\.]{1,63}\.iam\.gserviceaccount\.com$`)
	// A regex for a service account automatically created and maintained by GCP.
	gcpAgentSvcAcctRegex = regexp.MustCompile(`^` + gcpProjectIDRegexPart + `@[a-z]+.gserviceaccount.com$`)
)

// Definitions for all the custom functions that we add to CEL.
var celFuncs = []cel.EnvOption{
	// A split method on strings. Example: "foo,bar".split(",")
	//
	// Design decision: use a method instead of a plain function, because
	// that is the convention that the community seems to prefer for string
	// splitting. This is based on some quick googling of other peoples'
	// custom split functions.
	cel.Function(
		"split",
		cel.MemberOverload(
			"string_split",
			[]*cel.Type{cel.StringType, cel.StringType},
			types.NewListType(types.StringType),
			cel.BinaryBinding(func(lhs, rhs ref.Val) ref.Val {
				toSplit, ok := lhs.Value().(string)
				if !ok {
					return types.NewErr("internal error: lhs was %T but should have been a string", lhs.Value())
				}
				splitOn, ok := rhs.Value().(string)
				if !ok {
					return types.NewErr("internal error: rhs was %T but should have been a string", rhs.Value())
				}
				tokens := strings.Split(toSplit, splitOn)
				return types.NewStringList(celRegistry, tokens)
			}),
		),
	),

	// gcp_matches_service_account returns whether the input matches a full GCP
	// service account name. It can be either an API-created service account or
	// a platform-created service agent.
	//
	// You might want to use this for a template that requires a reference to an
	// already-created service account.
	//
	// Example:
	//   gcp_matches_service_account("platform-ops@abcxyz-my-project.iam.gserviceaccount.com")==true
	cel.Function(
		"gcp_matches_service_account",
		cel.Overload(
			"gcp_matches_service_account",
			[]*types.Type{types.StringType},
			cel.BoolType,
			cel.UnaryBinding(func(input ref.Val) ref.Val {
				asStr, ok := input.Value().(string)
				if !ok {
					return types.NewErr("internal error: argument was %T but should have been a string", input.Value())
				}
				// Design decision: use a sequence of two regex matches
				// rather writing one single nightmare regex that combines
				// both cases.
				return types.Bool(gcpCreatedSvcAcctRegex.MatchString(asStr) || gcpAgentSvcAcctRegex.MatchString(asStr))
			}),
		),
	),

	// gcp_matches_service_account_id returns whether the input matches the part
	// of a GCP service account name before the "@" sign.
	//
	// You might want to use this for a template that creates a service account.
	//
	// Example:
	//   gcp_matches_service_account_id("platform-ops")==true
	cel.Function(
		"gcp_matches_service_account_id",
		cel.Overload(
			"gcp_matches_service_account_id",
			[]*types.Type{types.StringType},
			cel.BoolType,
			cel.UnaryBinding(func(input ref.Val) ref.Val {
				asStr, ok := input.Value().(string)
				if !ok {
					return types.NewErr("internal error: argument was %T but should have been a string", input.Value())
				}
				return types.Bool(gcpSvcAcctIDRegex.MatchString(asStr))
			}),
		),
	),

	// gcp_matches_project_id returns whether the input matches the format of a
	// GCP project ID.
	//
	// You might want to use this for a template that creates a project or references
	// an existing project.
	//
	// Examples:
	//   gcp_matches_project_id("my-project")==true
	//   gcp_matches_project_id("example.com:my-project")==true
	cel.Function(
		"gcp_matches_project_id",
		cel.Overload(
			"gcp_matches_project_id",
			[]*types.Type{types.StringType},
			cel.BoolType,
			cel.UnaryBinding(func(input ref.Val) ref.Val {
				asStr, ok := input.Value().(string)
				if !ok {
					return types.NewErr("internal error: argument was %T but should have been a string", input.Value())
				}
				return types.Bool(gcpProjectIDRegex.MatchString(asStr))
			}),
		),
	),

	// gcp_matches_project_number returns whether the input matches the format
	// of a GCP project number (only digits, and not a tiny number).
	//
	// Examples:
	//   gcp_matches_project_number("123456789")==true
	//   gcp_matches_project_number("123abc")==false
	cel.Function(
		"gcp_matches_project_number",

		// There are multiple implementations; this one accepts string.
		cel.Overload("gcp_matches_project_number_string",
			[]*types.Type{types.StringType},
			cel.BoolType,
			cel.UnaryBinding(func(input ref.Val) ref.Val {
				strAny, err := input.ConvertToNative(reflect.TypeOf(""))
				if err != nil {
					return types.NewErr("internal error: argument was not convertible to string: %w", err)
				}
				str, ok := strAny.(string)
				if !ok {
					return types.NewErr("internal error: argument was %T but should have been a string", strAny)
				}

				u64, err := strconv.ParseUint(str, 10, 64)
				if err != nil {
					return types.Bool(false) // A string that's not parseable as a uint is not a valid project number
				}
				return types.Bool(u64 >= minProjectNum)
			}),
		),

		// There are multiple implementations; this one accepts int.
		cel.Overload("gcp_matches_project_number_int",
			[]*types.Type{types.IntType},
			cel.BoolType,
			cel.UnaryBinding(func(input ref.Val) ref.Val {
				u64Any, err := input.ConvertToNative(reflect.TypeOf(uint64(0)))
				if err != nil {
					return types.Bool(false)
				}
				u64, ok := u64Any.(uint64)
				if !ok {
					return types.NewErr(`internal error: "any" was %T but should have been uint64`, u64Any)
				}
				return types.Bool(u64 >= minProjectNum)
			}),
		),
	),
}

// celCompileAndEval parses, compiles, and executes the given CEL expr with the
// given variables in scope.
//
// The output of CEL execution is written into the location pointed to by
// outPtr. It must be a pointer. If the output of the CEL expression can't be
// converted to the given type, then an error will be returned. For example, if
// the CEL expression is "hello" and outPtr points to an int, an error will
// returned because CEL cannot treat "hello" as an integer.
func CelCompileAndEval(ctx context.Context, scope *Scope, expr model.String, outPtr any) error {
	prog, err := celCompile(ctx, scope, expr)
	if err != nil {
		return err //nolint:wrapcheck
	}
	if err := celEval(ctx, scope, expr.Pos, prog, outPtr); err != nil {
		return err
	}
	return nil
}

// celCompile parses and compiles the given expr into executable Program.
func celCompile(ctx context.Context, scope *Scope, expr model.String) (cel.Program, error) {
	startedAt := time.Now()

	celOpts := []cel.EnvOption{}
	for varName := range scope.All() {
		celOpts = append(celOpts, cel.Variable(varName, cel.StringType))
	}
	celOpts = append(celOpts, celFuncs...) // Add custom function bindings

	env, err := cel.NewEnv(celOpts...)
	if err != nil {
		return nil, expr.Pos.Errorf("internal error: failed configuring CEL environment: %w", err)
	}

	ast, issues := env.Compile(expr.Val)
	if err := issues.Err(); err != nil {
		return nil, expr.Pos.Errorf("failed compiling CEL expression: %w", err)
	}

	prog, err := env.Program(ast)
	if err != nil {
		return nil, expr.Pos.Errorf("failed constructing CEL program: %w", err)
	}

	latency := time.Since(startedAt)
	logger := logging.FromContext(ctx).With("logger", "celCompile")
	logger.DebugContext(ctx, "cel compilation time",
		"duration_usec", latency.Microseconds(),
		"duration_human", latency.String())

	return prog, nil
}

// celEval runs a previously-compiled CEL Program (which you can get from
// celCompile()).
//
// The output of CEL execution is written into the location pointed to by
// outPtr. It must be a pointer. If the output of the CEL expression can't be
// converted to the given type, then an error will be returned. For example, if
// the CEL expression is "hello" and outPtr points to an int, an error will
// returned because CEL cannot treat "hello" as an integer.
func celEval(ctx context.Context, scope *Scope, pos *model.ConfigPos, prog cel.Program, outPtr any) error {
	startedAt := time.Now()

	// The CEL engine needs variable values as a map[string]any, but we have a
	// map[string]string, so convert.
	scopeAll := scope.All()
	scopeMapAny := make(map[string]any, len(scopeAll))
	for varName, varVal := range scopeAll {
		scopeMapAny[varName] = varVal
	}

	celOut, _, err := prog.Eval(scopeMapAny)
	if err != nil {
		return pos.Errorf("failed executing CEL expression: %w", err)
	}

	outPtrRefVal := reflect.ValueOf(outPtr)
	if kind := outPtrRefVal.Kind(); kind != reflect.Pointer {
		return fmt.Errorf("internal error: celEval must be provided a pointer, but got a %s", kind)
	}

	outRefVal := outPtrRefVal.Elem()

	celAny, err := celOut.ConvertToNative(outRefVal.Type())
	if err != nil {
		return pos.Errorf("CEL expression result couldn't be converted to %s. The CEL engine error was: %w", outRefVal.Type(), err)
	}

	outRefVal.Set(reflect.ValueOf(celAny))

	latency := time.Since(startedAt)
	logger := logging.FromContext(ctx).With("logger", "celEval")
	logger.DebugContext(ctx, "cel evaluation time",
		"duration_usec", latency.Microseconds(),
		"duration_human", latency.String())

	return nil
}
