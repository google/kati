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

	"github.com/golang/glog"
)

// DepNode represents a makefile rule for an output.
type DepNode struct {
	Output             string
	Cmds               []string
	Deps               []*DepNode
	OrderOnlys         []*DepNode
	Parents            []*DepNode
	HasRule            bool
	IsPhony            bool
	ActualInputs       []string
	TargetSpecificVars Vars
	Filename           string
	Lineno             int
}

func (n *DepNode) String() string {
	return fmt.Sprintf("Dep{output=%s cmds=%d deps=%d orders=%d hasRule=%t phony=%t filename=%s lineno=%d}",
		n.Output, len(n.Cmds), len(n.Deps), len(n.OrderOnlys), n.HasRule, n.IsPhony, n.Filename, n.Lineno)
}

type depBuilder struct {
	rules    map[string]*rule
	ruleVars map[string]Vars

	implicitRules *ruleTrie

	suffixRules map[string][]*rule
	firstRule   *rule
	vars        Vars
	ev          *Evaluator
	vpaths      searchPaths
	done        map[string]*DepNode
	phony       map[string]bool

	trace                         []string
	nodeCnt                       int
	pickExplicitRuleCnt           int
	pickImplicitRuleCnt           int
	pickSuffixRuleCnt             int
	pickExplicitRuleWithoutCmdCnt int
}

type ruleTrieEntry struct {
	rule   *rule
	suffix string
}

type ruleTrie struct {
	rules    []ruleTrieEntry
	children map[byte]*ruleTrie
}

func newRuleTrie() *ruleTrie {
	return &ruleTrie{
		children: make(map[byte]*ruleTrie),
	}
}

func (rt *ruleTrie) add(name string, r *rule) {
	glog.V(1).Infof("rule trie: add %q %v %s", name, r.outputPatterns[0], r)
	if name == "" || name[0] == '%' {
		glog.V(1).Infof("rule trie: add entry %q %v %s", name, r.outputPatterns[0], r)
		rt.rules = append(rt.rules, ruleTrieEntry{
			rule:   r,
			suffix: name,
		})
		return
	}
	c, found := rt.children[name[0]]
	if !found {
		c = newRuleTrie()
		rt.children[name[0]] = c
	}
	c.add(name[1:], r)
}

func (rt *ruleTrie) lookup(name string) []*rule {
	glog.V(1).Infof("rule trie: lookup %q", name)
	if rt == nil {
		return nil
	}
	var rules []*rule
	for _, entry := range rt.rules {
		if (entry.suffix == "" && name == "") || strings.HasSuffix(name, entry.suffix[1:]) {
			rules = append(rules, entry.rule)
		}
	}
	if name == "" {
		return rules
	}
	rules = append(rules, rt.children[name[0]].lookup(name[1:])...)
	glog.V(1).Infof("rule trie: lookup %q => %v", name, rules)
	return rules
}

func (rt *ruleTrie) size() int {
	if rt == nil {
		return 0
	}
	size := len(rt.rules)
	for _, c := range rt.children {
		size += c.size()
	}
	return size
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
	_, ok := db.vpaths.exists(target)
	return ok
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
		// TODO(ukai): should return error?
		panic(fmt.Sprintf("FIXME: Implicit rule should have only one output but %q", outputs))
	}
	glog.V(1).Infof("merge? %q", db.ruleVars)
	glog.V(1).Infof("merge? %q", outputs[0])
	ivars, present := db.ruleVars[outputs[0]]
	if !present {
		return vars
	}
	if vars == nil {
		return ivars
	}
	glog.V(1).Info("merge!")
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

	irules := db.implicitRules.lookup(output)
	for i := len(irules) - 1; i >= 0; i-- {
		irule := irules[i]
		if !db.canPickImplicitRule(irule, output) {
			glog.Infof("ignore implicit rule %q %s", output, irule)
			continue
		}
		glog.Infof("pick implicit rule %q => %q %s", output, irule.outputPatterns, irule)
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
			// TODO(ukai): should return error?
			panic(fmt.Sprintf("FIXME: unexpected number of input for a suffix rule (%d)", len(irule.inputs)))
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

func expandInputs(rule *rule, output string) []string {
	var inputs []string
	for _, input := range rule.inputs {
		if len(rule.outputPatterns) > 0 {
			if len(rule.outputPatterns) != 1 {
				panic(fmt.Sprintf("FIXME: multiple output pattern is not supported yet"))
			}
			input = intern(rule.outputPatterns[0].subst(input, output))
		} else if rule.isSuffixRule {
			input = intern(replaceSuffix(output, input))
		}
		inputs = append(inputs, input)
	}
	return inputs
}

func (db *depBuilder) buildPlan(output string, neededBy string, tsvs Vars) (*DepNode, error) {
	glog.V(1).Infof("Evaluating command: %s", output)
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
					var err error
					v, err = oldVar.AppendVar(db.ev, tsv)
					if err != nil {
						return nil, err
					}
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

	inputs := expandInputs(rule, output)
	glog.Infof("Evaluating command: %s inputs:%q => %q", output, rule.inputs, inputs)
	for _, input := range inputs {
		db.trace = append(db.trace, input)
		ni, err := db.buildPlan(input, output, tsvs)
		db.trace = db.trace[0 : len(db.trace)-1]
		if err != nil {
			return nil, err
		}
		if ni != nil {
			n.Deps = append(n.Deps, ni)
			ni.Parents = append(ni.Parents, n)
		}
	}

	for _, input := range rule.orderOnlyInputs {
		db.trace = append(db.trace, input)
		ni, err := db.buildPlan(input, output, tsvs)
		db.trace = db.trace[0 : len(db.trace)-1]
		if err != nil {
			return nil, err
		}
		if n != nil {
			n.OrderOnlys = append(n.OrderOnlys, ni)
			ni.Parents = append(ni.Parents, n)
		}
	}

	n.HasRule = true
	n.Cmds = rule.cmds
	n.ActualInputs = inputs
	n.TargetSpecificVars = make(Vars)
	for k, v := range tsvs {
		if glog.V(1) {
			glog.Infof("output=%s tsv %s=%s", output, k, v)
		}
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

func mergeRules(oldRule, r *rule, output string, isSuffixRule bool) (*rule, error) {
	if oldRule.isDoubleColon != r.isDoubleColon {
		return nil, r.errorf("*** target file %q has both : and :: entries.", output)
	}
	if len(oldRule.cmds) > 0 && len(r.cmds) > 0 && !isSuffixRule && !r.isDoubleColon {
		warn(r.cmdpos(), "overriding commands for target %q", output)
		warn(oldRule.cmdpos(), "ignoring old commands for target %q", output)
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
	return mr, nil
}

// expandPattern expands static pattern (target: target-pattern: prereq-pattern).

func expandPattern(r *rule) []*rule {
	if len(r.outputs) == 0 {
		return []*rule{r}
	}
	if len(r.outputPatterns) != 1 {
		return []*rule{r}
	}
	var rules []*rule
	pat := r.outputPatterns[0]
	for _, output := range r.outputs {
		nr := new(rule)
		*nr = *r
		nr.outputs = []string{output}
		nr.outputPatterns = nil
		nr.inputs = nil
		for _, input := range r.inputs {
			nr.inputs = append(nr.inputs, intern(pat.subst(input, output)))
		}
		rules = append(rules, nr)
	}
	glog.V(1).Infof("expand static pattern: outputs=%q inputs=%q -> %q", r.outputs, r.inputs, rules)
	return rules
}

func (db *depBuilder) populateExplicitRule(r *rule) error {
	// It seems rules with no outputs are siliently ignored.
	if len(r.outputs) == 0 {
		return nil
	}
	for _, output := range r.outputs {
		output = trimLeadingCurdir(output)

		isSuffixRule := db.populateSuffixRule(r, output)

		if oldRule, present := db.rules[output]; present {
			mr, err := mergeRules(oldRule, r, output, isSuffixRule)
			if err != nil {
				return err
			}
			db.rules[output] = mr
		} else {
			db.rules[output] = r
			if db.firstRule == nil && !strings.HasPrefix(output, ".") {
				db.firstRule = r
			}
		}
	}
	return nil
}

func (db *depBuilder) populateImplicitRule(r *rule) {
	for _, outputPattern := range r.outputPatterns {
		ir := &rule{}
		*ir = *r
		ir.outputPatterns = []pattern{outputPattern}
		db.implicitRules.add(outputPattern.String(), ir)
	}
}

func (db *depBuilder) populateRules(er *evalResult) error {
	for _, r := range er.rules {
		for i, input := range r.inputs {
			r.inputs[i] = trimLeadingCurdir(input)
		}
		for i, orderOnlyInput := range r.orderOnlyInputs {
			r.orderOnlyInputs[i] = trimLeadingCurdir(orderOnlyInput)
		}
		for _, r := range expandPattern(r) {
			err := db.populateExplicitRule(r)
			if err != nil {
				return err
			}
			if len(r.outputs) == 0 {
				db.populateImplicitRule(r)
			}
		}
	}
	return nil
}

func (db *depBuilder) reportStats() {
	if !PeriodicStatsFlag {
		return
	}

	logStats("node=%d explicit=%d implicit=%d suffix=%d explicitWOCmd=%d",
		db.nodeCnt, db.pickExplicitRuleCnt, db.pickImplicitRuleCnt, db.pickSuffixRuleCnt, db.pickExplicitRuleWithoutCmdCnt)
	if len(db.trace) > 1 {
		logStats("trace=%q", db.trace)
	}
}

func newDepBuilder(er *evalResult, vars Vars) (*depBuilder, error) {
	db := &depBuilder{
		rules:         make(map[string]*rule),
		ruleVars:      er.ruleVars,
		implicitRules: newRuleTrie(),
		suffixRules:   make(map[string][]*rule),
		vars:          vars,
		ev:            NewEvaluator(vars),
		vpaths:        er.vpaths,
		done:          make(map[string]*DepNode),
		phony:         make(map[string]bool),
	}

	err := db.populateRules(er)
	if err != nil {
		return nil, err
	}
	rule, present := db.rules[".PHONY"]
	if present {
		for _, input := range rule.inputs {
			db.phony[input] = true
		}
	}
	return db, nil
}

func (db *depBuilder) Eval(targets []string) ([]*DepNode, error) {
	if len(targets) == 0 {
		if db.firstRule == nil {
			return nil, fmt.Errorf("*** No targets.")
		}
		targets = append(targets, db.firstRule.outputs[0])
		var phonys []string
		for t := range db.phony {
			phonys = append(phonys, t)
		}
		sort.Strings(phonys)
		targets = append(targets, phonys...)
	}

	if StatsFlag {
		logStats("%d variables", len(db.vars))
		logStats("%d explicit rules", len(db.rules))
		logStats("%d implicit rules", db.implicitRules.size())
		logStats("%d suffix rules", len(db.suffixRules))
		logStats("%d dirs %d files", fsCache.dirs(), fsCache.files())
	}

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
