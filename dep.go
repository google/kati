// Copyright 2015 Google Inc. All rights reserved
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

package kati

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type DepNode struct {
	Output             string
	Cmds               []string
	Deps               []*DepNode
	Parents            []*DepNode
	HasRule            bool
	IsOrderOnly        bool
	IsPhony            bool
	ActualInputs       []string
	TargetSpecificVars Vars
	Filename           string
	Lineno             int
}

func (n *DepNode) String() string {
	return fmt.Sprintf("Dep{output=%s cmds=%d deps=%d hasRule=%t orderOnly=%t, phony=%t filename=%s lineno=%d}",
		n.Output, len(n.Cmds), len(n.Deps), n.HasRule, n.IsOrderOnly, n.IsPhony, n.Filename, n.Lineno)
}

type depBuilder struct {
	rules    map[string]*rule
	ruleVars map[string]Vars

	implicitRules []*rule // pattern=%. no prefix,suffix.
	iprefixRules  []*rule // pattern=prefix%..  may have suffix
	isuffixRules  []*rule // pattern=%suffix  no prefix

	suffixRules map[string][]*rule
	firstRule   *rule
	vars        Vars
	done        map[string]*DepNode
	phony       map[string]bool

	trace                         []string
	nodeCnt                       int
	pickExplicitRuleCnt           int
	pickImplicitRuleCnt           int
	pickSuffixRuleCnt             int
	pickExplicitRuleWithoutCmdCnt int
}

func replaceSuffix(s string, newsuf string) string {
	// TODO: Factor out the logic around suffix rules and use
	// it from substitution references.
	// http://www.gnu.org/software/make/manual/make.html#Substitution-Refs
	return fmt.Sprintf("%s.%s", stripExt(s), newsuf)
}

func (db *depBuilder) exists(target string) bool {
	_, present := db.rules[target]
	if present {
		return true
	}
	if db.phony[target] {
		return true
	}
	return exists(target)
}

func (db *depBuilder) PickImplicitRules(output string) []*rule {
	var rules []*rule
	i := sort.Search(len(db.iprefixRules), func(i int) bool {
		prefix := db.iprefixRules[i].outputPatterns[0].prefix
		if strings.HasPrefix(output, prefix) {
			return true
		}
		return prefix >= output
	})
	if i < len(db.iprefixRules) {
		for ; i < len(db.iprefixRules); i++ {
			rule := db.iprefixRules[i]
			if !strings.HasPrefix(output, rule.outputPatterns[0].prefix) {
				break
			}
			if db.canPickImplicitRule(rule, output) {
				rules = append(rules, rule)
			}
		}
	}

	i = sort.Search(len(db.isuffixRules), func(i int) bool {
		suffix := db.isuffixRules[i].outputPatterns[0].suffix
		if strings.HasSuffix(output, suffix) {
			return true
		}
		return reverse(suffix) >= reverse(output)
	})
	if i < len(db.isuffixRules) {
		for ; i < len(db.isuffixRules); i++ {
			rule := db.isuffixRules[i]
			if !strings.HasSuffix(output, rule.outputPatterns[0].suffix) {
				break
			}
			if db.canPickImplicitRule(rule, output) {
				rules = append(rules, rule)
			}
		}
	}
	for _, rule := range db.implicitRules {
		if db.canPickImplicitRule(rule, output) {
			rules = append(rules, rule)
		}
	}
	// TODO(ukai): which implicit rules is selected?
	// longest match? last defined?
	return rules
}

func (db *depBuilder) canPickImplicitRule(r *rule, output string) bool {
	outputPattern := r.outputPatterns[0]
	if !outputPattern.match(output) {
		return false
	}
	for _, input := range r.inputs {
		input = outputPattern.subst(input, output)
		if !db.exists(input) {
			return false
		}
	}
	return true
}

func (db *depBuilder) mergeImplicitRuleVars(outputs []string, vars Vars) Vars {
	if len(outputs) != 1 {
		panic(fmt.Sprintf("Implicit rule should have only one output but %q", outputs))
	}
	logf("merge? %q", db.ruleVars)
	logf("merge? %q", outputs[0])
	ivars, present := db.ruleVars[outputs[0]]
	if !present {
		return vars
	}
	if vars == nil {
		return ivars
	}
	logf("merge!")
	v := make(Vars)
	v.Merge(ivars)
	v.Merge(vars)
	return v
}

func (db *depBuilder) pickRule(output string) (*rule, Vars, bool) {
	r, present := db.rules[output]
	vars := db.ruleVars[output]
	if present {
		db.pickExplicitRuleCnt++
		if len(r.cmds) > 0 {
			return r, vars, true
		}
		// If none of the explicit rules for a target has commands,
		// then `make' searches for an applicable implicit rule to
		// find some commands.
		db.pickExplicitRuleWithoutCmdCnt++
	}

	for _, irule := range db.PickImplicitRules(output) {
		db.pickImplicitRuleCnt++
		if r != nil {
			ir := &rule{}
			*ir = *r
			ir.outputPatterns = irule.outputPatterns
			// implicit rule's prerequisites will be used for $<
			ir.inputs = append(irule.inputs, ir.inputs...)
			ir.cmds = irule.cmds
			// TODO(ukai): filename, lineno?
			ir.cmdLineno = irule.cmdLineno
			return ir, vars, true
		}
		if vars != nil {
			var outputs []string
			for _, op := range irule.outputPatterns {
				outputs = append(outputs, op.String())
			}
			vars = db.mergeImplicitRuleVars(outputs, vars)
		}
		// TODO(ukai): check len(irule.cmd) ?
		return irule, vars, true
	}

	outputSuffix := filepath.Ext(output)
	if !strings.HasPrefix(outputSuffix, ".") {
		return r, vars, r != nil
	}
	rules, present := db.suffixRules[outputSuffix[1:]]
	if !present {
		return r, vars, r != nil
	}
	for _, irule := range rules {
		if len(irule.inputs) != 1 {
			panic(fmt.Sprintf("unexpected number of input for a suffix rule (%d)", len(irule.inputs)))
		}
		if !db.exists(replaceSuffix(output, irule.inputs[0])) {
			continue
		}
		db.pickSuffixRuleCnt++
		if r != nil {
			sr := &rule{}
			*sr = *r
			// TODO(ukai): input order is correct?
			sr.inputs = append([]string{replaceSuffix(output, irule.inputs[0])}, r.inputs...)
			sr.cmds = irule.cmds
			// TODO(ukai): filename, lineno?
			sr.cmdLineno = irule.cmdLineno
			return sr, vars, true
		}
		if vars != nil {
			vars = db.mergeImplicitRuleVars(irule.outputs, vars)
		}
		// TODO(ukai): check len(irule.cmd) ?
		return irule, vars, true
	}
	return r, vars, r != nil
}

func (db *depBuilder) buildPlan(output string, neededBy string, tsvs Vars) (*DepNode, error) {
	logf("Evaluating command: %s", output)
	db.nodeCnt++
	if db.nodeCnt%100 == 0 {
		db.reportStats()
	}

	if n, present := db.done[output]; present {
		return n, nil
	}

	n := &DepNode{Output: output, IsPhony: db.phony[output]}
	db.done[output] = n

	// create depnode for phony targets?
	rule, vars, present := db.pickRule(output)
	if !present {
		return n, nil
	}

	var restores []func()
	if vars != nil {
		for name, v := range vars {
			// TODO: Consider not updating db.vars.
			tsv := v.(*targetSpecificVar)
			restores = append(restores, db.vars.save(name))
			restores = append(restores, tsvs.save(name))
			switch tsv.op {
			case ":=", "=":
				db.vars[name] = tsv
				tsvs[name] = v
			case "+=":
				oldVar, present := db.vars[name]
				if !present || oldVar.String() == "" {
					db.vars[name] = tsv
				} else {
					v = oldVar.AppendVar(NewEvaluator(db.vars), tsv)
					db.vars[name] = v
				}
				tsvs[name] = v
			case "?=":
				if _, present := db.vars[name]; !present {
					db.vars[name] = tsv
					tsvs[name] = v
				}
			}
		}
		defer func() {
			for _, restore := range restores {
				restore()
			}
		}()
	}

	var children []*DepNode
	var actualInputs []string
	logf("Evaluating command: %s inputs:%q", output, rule.inputs)
	for _, input := range rule.inputs {
		if len(rule.outputPatterns) > 0 {
			if len(rule.outputPatterns) > 1 {
				panic("TODO: multiple output pattern is not supported yet")
			}
			input = intern(rule.outputPatterns[0].subst(input, output))
		} else if rule.isSuffixRule {
			input = intern(replaceSuffix(output, input))
		}
		actualInputs = append(actualInputs, input)

		db.trace = append(db.trace, input)
		n, err := db.buildPlan(input, output, tsvs)
		db.trace = db.trace[0 : len(db.trace)-1]
		if err != nil {
			return nil, err
		}
		if n != nil {
			children = append(children, n)
		}
	}

	for _, input := range rule.orderOnlyInputs {
		db.trace = append(db.trace, input)
		n, err := db.buildPlan(input, output, tsvs)
		db.trace = db.trace[0 : len(db.trace)-1]
		if err != nil {
			return nil, err
		}
		if n != nil {
			n.IsOrderOnly = true
			children = append(children, n)
		}
	}

	n.Deps = children
	for _, c := range n.Deps {
		c.Parents = append(c.Parents, n)
	}
	n.HasRule = true
	n.Cmds = rule.cmds
	n.ActualInputs = actualInputs
	n.TargetSpecificVars = make(Vars)
	for k, v := range tsvs {
		n.TargetSpecificVars[k] = v
	}
	n.Filename = rule.filename
	if len(rule.cmds) > 0 {
		if rule.cmdLineno > 0 {
			n.Lineno = rule.cmdLineno
		} else {
			n.Lineno = rule.lineno
		}
	}
	return n, nil
}

func (db *depBuilder) populateSuffixRule(r *rule, output string) bool {
	if len(output) == 0 || output[0] != '.' {
		return false
	}
	rest := output[1:]
	dotIndex := strings.IndexByte(rest, '.')
	// If there is only a single dot or the third dot, this is not a
	// suffix rule.
	if dotIndex < 0 || strings.IndexByte(rest[dotIndex+1:], '.') >= 0 {
		return false
	}

	// This is a suffix rule.
	inputSuffix := rest[:dotIndex]
	outputSuffix := rest[dotIndex+1:]
	sr := &rule{}
	*sr = *r
	sr.inputs = []string{inputSuffix}
	sr.isSuffixRule = true
	db.suffixRules[outputSuffix] = append([]*rule{sr}, db.suffixRules[outputSuffix]...)
	return true
}

func mergeRules(oldRule, r *rule, output string, isSuffixRule bool) *rule {
	if oldRule.isDoubleColon != r.isDoubleColon {
		errorExit(r.filename, r.lineno, "*** target file %q has both : and :: entries.", output)
	}
	if len(oldRule.cmds) > 0 && len(r.cmds) > 0 && !isSuffixRule && !r.isDoubleColon {
		warn(r.filename, r.cmdLineno, "overriding commands for target %q", output)
		warn(oldRule.filename, oldRule.cmdLineno, "ignoring old commands for target %q", output)
	}

	mr := &rule{}
	*mr = *r
	if r.isDoubleColon {
		mr.cmds = append(oldRule.cmds, mr.cmds...)
	} else if len(oldRule.cmds) > 0 && len(r.cmds) == 0 {
		mr.cmds = oldRule.cmds
	}
	// If the latter rule has a command (regardless of the
	// commands in oldRule), inputs in the latter rule has a
	// priority.
	if len(r.cmds) > 0 {
		mr.inputs = append(mr.inputs, oldRule.inputs...)
		mr.orderOnlyInputs = append(mr.orderOnlyInputs, oldRule.orderOnlyInputs...)
	} else {
		mr.inputs = append(oldRule.inputs, mr.inputs...)
		mr.orderOnlyInputs = append(oldRule.orderOnlyInputs, mr.orderOnlyInputs...)
	}
	mr.outputPatterns = append(mr.outputPatterns, oldRule.outputPatterns...)
	return mr
}

func (db *depBuilder) populateExplicitRule(r *rule) {
	// It seems rules with no outputs are siliently ignored.
	if len(r.outputs) == 0 {
		return
	}
	for _, output := range r.outputs {
		output = trimLeadingCurdir(output)

		isSuffixRule := db.populateSuffixRule(r, output)

		if oldRule, present := db.rules[output]; present {
			mr := mergeRules(oldRule, r, output, isSuffixRule)
			db.rules[output] = mr
		} else {
			db.rules[output] = r
			if db.firstRule == nil && !strings.HasPrefix(output, ".") {
				db.firstRule = r
			}
		}
	}
}

func (db *depBuilder) populateImplicitRule(r *rule) {
	for _, outputPattern := range r.outputPatterns {
		ir := &rule{}
		*ir = *r
		ir.outputPatterns = []pattern{outputPattern}
		if outputPattern.prefix != "" {
			db.iprefixRules = append(db.iprefixRules, ir)
		} else if outputPattern.suffix != "" {
			db.isuffixRules = append(db.isuffixRules, ir)
		} else {
			db.implicitRules = append(db.implicitRules, ir)
		}
	}
}

func (db *depBuilder) populateRules(er *evalResult) {
	for _, r := range er.rules {
		for i, input := range r.inputs {
			r.inputs[i] = trimLeadingCurdir(input)
		}
		for i, orderOnlyInput := range r.orderOnlyInputs {
			r.orderOnlyInputs[i] = trimLeadingCurdir(orderOnlyInput)
		}
		db.populateExplicitRule(r)

		if len(r.outputs) == 0 {
			db.populateImplicitRule(r)
		}
	}

	// reverse to the last implicit rules should be selected.
	// testcase/implicit_pattern_rule.mk
	reverseImplicitRules(db.implicitRules)
	reverseImplicitRules(db.iprefixRules)
	reverseImplicitRules(db.isuffixRules)

	sort.Stable(byPrefix(db.iprefixRules))
	sort.Stable(bySuffix(db.isuffixRules))
}

func reverseImplicitRules(rules []*rule) {
	for i := 0; i < len(rules)/2; i++ {
		j := len(rules) - i - 1
		rules[i], rules[j] = rules[j], rules[i]
	}
}

type byPrefix []*rule

func (p byPrefix) Len() int      { return len(p) }
func (p byPrefix) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p byPrefix) Less(i, j int) bool {
	return p[i].outputPatterns[0].prefix < p[j].outputPatterns[0].prefix
}

type bySuffix []*rule

func (s bySuffix) Len() int      { return len(s) }
func (s bySuffix) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s bySuffix) Less(i, j int) bool {
	return reverse(s[i].outputPatterns[0].suffix) < reverse(s[j].outputPatterns[0].suffix)
}

func (db *depBuilder) reportStats() {
	if !LogFlag && !PeriodicStatsFlag {
		return
	}

	logStats("node=%d explicit=%d implicit=%d suffix=%d explicitWOCmd=%d",
		db.nodeCnt, db.pickExplicitRuleCnt, db.pickImplicitRuleCnt, db.pickSuffixRuleCnt, db.pickExplicitRuleWithoutCmdCnt)
	if len(db.trace) > 1 {
		logStats("trace=%q", db.trace)
	}
}

func newDepBuilder(er *evalResult, vars Vars) *depBuilder {
	db := &depBuilder{
		rules:       make(map[string]*rule),
		ruleVars:    er.ruleVars,
		suffixRules: make(map[string][]*rule),
		vars:        vars,
		done:        make(map[string]*DepNode),
		phony:       make(map[string]bool),
	}

	db.populateRules(er)
	rule, present := db.rules[".PHONY"]
	if present {
		for _, input := range rule.inputs {
			db.phony[input] = true
		}
	}
	return db
}

func (db *depBuilder) Eval(targets []string) ([]*DepNode, error) {
	if len(targets) == 0 {
		if db.firstRule == nil {
			errorNoLocationExit("*** No targets.")
		}
		targets = append(targets, db.firstRule.outputs[0])
	}

	logStats("%d variables", len(db.vars))
	logStats("%d explicit rules", len(db.rules))
	logStats("%d implicit rules", len(db.implicitRules))
	logStats("%d suffix rules", len(db.suffixRules))

	var nodes []*DepNode
	for _, target := range targets {
		db.trace = []string{target}
		n, err := db.buildPlan(target, "", make(Vars))
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	db.reportStats()
	return nodes, nil
}
