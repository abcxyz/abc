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

package commands

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/pkg/logging"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

var (
	celRegistry = types.NewEmptyRegistry()

	celFuncs = []cel.EnvOption{
		// Adds a split method on strings. Example: "foo,bar".split(",")
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
	}
)

// celCompile parses and compiles the given expr into executable Program.
func celCompile(ctx context.Context, scope *scope, expr model.String) (cel.Program, error) {
	startedAt := time.Now()

	celOpts := []cel.EnvOption{}
	for varName := range scope.All() {
		celOpts = append(celOpts, cel.Variable(varName, cel.StringType))
	}
	celOpts = append(celOpts, celFuncs...) // Add custom function bindings

	env, err := cel.NewEnv(celOpts...)
	if err != nil {
		return nil, expr.Pos.AnnotateErr(fmt.Errorf("internal error: failed configuring CEL environment: %w", err)) //nolint:wrapcheck
	}

	ast, issues := env.Compile(expr.Val)
	if err := issues.Err(); err != nil {
		return nil, expr.Pos.AnnotateErr(fmt.Errorf("failed compiling CEL expression: %w", err)) //nolint:wrapcheck
	}

	prog, err := env.Program(ast)
	if err != nil {
		return nil, expr.Pos.AnnotateErr(fmt.Errorf("failed constructing CEL program: %w", err)) //nolint:wrapcheck
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
func celEval(ctx context.Context, scope *scope, pos *model.ConfigPos, prog cel.Program, outPtr any) error {
	startedAt := time.Now()

	// The CEL engine needs variable values as a map[string]any, but we have a
	// map[string]string, so convert.
	scopeMapAny := make(map[string]any, len(scope.All()))
	for varName, varVal := range scope.All() {
		scopeMapAny[varName] = varVal
	}

	celOut, _, err := prog.Eval(scopeMapAny)
	if err != nil {
		return pos.AnnotateErr(fmt.Errorf("failed executing CEL expression: %w", err)) //nolint:wrapcheck
	}

	outPtrRefVal := reflect.ValueOf(outPtr)
	if kind := outPtrRefVal.Kind(); kind != reflect.Pointer {
		return fmt.Errorf("internal error: celEval must be provided a pointer, but got a %s", kind)
	}

	outRefVal := outPtrRefVal.Elem()

	celAny, err := celOut.ConvertToNative(outRefVal.Type())
	if err != nil {
		return pos.AnnotateErr(fmt.Errorf("CEL expression result couldn't be converted to %s. The CEL engine error was: %w", outRefVal.Type(), err)) //nolint:wrapcheck
	}

	outRefVal.Set(reflect.ValueOf(celAny))

	latency := time.Since(startedAt)
	logger := logging.FromContext(ctx).With("logger", "celEval")
	logger.DebugContext(ctx, "cel evaluation time",
		"duration_usec", latency.Microseconds(),
		"duration_human", latency.String())

	return nil
}
