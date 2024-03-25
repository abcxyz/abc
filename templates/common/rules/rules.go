// Copyright 2024 The Authors (see AUTHORS file)
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

// Package rules contains function for handling rule evaluation
package rules

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/abcxyz/abc/templates/common"
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta6"
)

// ValidateRules validates the given rules using the given context and scope.
// If any rules are violated, an error is returned.
func ValidateRules(ctx context.Context, scope *common.Scope, rules []*spec.Rule) error {
	sb := &strings.Builder{}
	tw := tabwriter.NewWriter(sb, 8, 0, 2, ' ', 0)

	ValidateRulesWithMessage(ctx, scope, rules, tw, func() {})

	tw.Flush()
	if sb.Len() > 0 {
		return fmt.Errorf("rules validation failed:\n%s", sb.String())
	}
	return nil
}

// ValidateRulesWithMessage validates the given rules using the given context and scope.
// If any rules are violated, the given preWriteCallBack function is called, and then
// the rule is written to the given writer.
func ValidateRulesWithMessage(ctx context.Context, scope *common.Scope, rules []*spec.Rule, writer *tabwriter.Writer, preWriteCallBack func()) {
	for _, rule := range rules {
		var ok bool
		err := common.CelCompileAndEval(ctx, scope, rule.Rule, &ok)
		if ok && err == nil {
			continue
		}

		preWriteCallBack()
		WriteRule(writer, rule, false, 0)
		if err != nil {
			fmt.Fprintf(writer, "\nCEL error:\t%s", err.Error())
		}
		fmt.Fprintf(writer, "\n") // Add vertical relief between validation messages
	}
}

// WriteRule writes a human-readable description of the given rules to the given
// writer in a 2-column format.
//
// Sometimes we run this in a context where we want to include the index of the
// rules in the list of rules; in that case, pass includeIndex=true and the index
// value. If includeIndex is false, then index is ignored.
func WriteRule(writer *tabwriter.Writer, rule *spec.Rule, includeIndex bool, index int) {
	indexStr := ""
	if includeIndex {
		indexStr = fmt.Sprintf(" %d", index)
	}

	fmt.Fprintf(writer, "\nRule%s:\t%s", indexStr, rule.Rule.Val)
	if rule.Message.Val != "" {
		fmt.Fprintf(writer, "\nRule%s msg:\t%s", indexStr, rule.Message.Val)
	}
}
