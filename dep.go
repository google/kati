package main

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

type DepBuilder struct {
	rules       map[string]*Rule
	ruleVars    map[string]Vars
	irules      map[string][]*Rule // output -> implicitRules
	suffixRules map[string][]*Rule
	firstRule   *Rule
	vars        Vars
	done        map[string]*DepNode
	phony       map[string]bool

	trace                         []string
	nImplicitRules                int
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

func (db *DepBuilder) exists(target string) bool {
	_, present := db.rules[target]
	if present {
		return true
	}
	if db.phony[target] {
		return true
	}
	return exists(target)
}

func (db *DepBuilder) canPickImplicitRule(rule *Rule, output string) bool {
	outputPattern := rule.outputPatterns[0]
	if !outputPattern.match(output) {
		return false
	}
	for _, input := range rule.inputs {
		input = outputPattern.subst(input, output)
		if !db.exists(input) {
			return false
		}
	}
	return true
}

func (db *DepBuilder) mergeImplicitRuleVars(outputs []string, vars Vars) Vars {
	if len(outputs) != 1 {
		panic(fmt.Sprintf("Implicit rule should have only one output but %q", outputs))
	}
	Log("merge? %q", db.ruleVars)
	Log("merge? %q", outputs[0])
	ivars, present := db.ruleVars[outputs[0]]
	if !present {
		return vars
	}
	if vars == nil {
		return ivars
	}
	Log("merge!")
	v := make(Vars)
	v.Merge(ivars)
	v.Merge(vars)
	return v
}

func (db *DepBuilder) pickRule(output string) (*Rule, Vars, bool) {
	rule, present := db.rules[output]
	vars := db.ruleVars[output]
	if present {
		db.pickExplicitRuleCnt++
		if len(rule.cmds) > 0 {
			return rule, vars, true
		}
		// If none of the explicit rules for a target has commands,
		// then `make' searches for an applicable implicit rule to
		// find some commands.
		db.pickExplicitRuleWithoutCmdCnt++
	}

	for _, irule := range db.irules[output] {
		if !db.canPickImplicitRule(irule, output) {
			continue
		}
		db.pickImplicitRuleCnt++
		if rule != nil {
			r := &Rule{}
			*r = *rule
			r.outputPatterns = irule.outputPatterns
			// implicit rule's prerequisites will be used for $<
			r.inputs = append(irule.inputs, r.inputs...)
			r.cmds = irule.cmds
			// TODO(ukai): filename, lineno?
			r.cmdLineno = irule.cmdLineno
			return r, vars, true
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
		return rule, vars, rule != nil
	}
	rules, present := db.suffixRules[outputSuffix[1:]]
	if !present {
		return rule, vars, rule != nil
	}
	for _, irule := range rules {
		if len(irule.inputs) != 1 {
			panic(fmt.Sprintf("unexpected number of input for a suffix rule (%d)", len(irule.inputs)))
		}
		if !db.exists(replaceSuffix(output, irule.inputs[0])) {
			continue
		}
		db.pickSuffixRuleCnt++
		if rule != nil {
			r := &Rule{}
			*r = *rule
			// TODO(ukai): input order is correct?
			r.inputs = append([]string{replaceSuffix(output, irule.inputs[0])}, r.inputs...)
			r.cmds = irule.cmds
			// TODO(ukai): filename, lineno?
			r.cmdLineno = irule.cmdLineno
			return r, vars, true
		}
		if vars != nil {
			vars = db.mergeImplicitRuleVars(irule.outputs, vars)
		}
		// TODO(ukai): check len(irule.cmd) ?
		return irule, vars, true
	}
	return rule, vars, rule != nil
}

func (db *DepBuilder) buildPlan(output string, neededBy string, tsvs Vars) (*DepNode, error) {
	Log("Evaluating command: %s", output)
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
			tsv := v.(TargetSpecificVar)
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
					v = oldVar.AppendVar(newEvaluator(db.vars), tsv)
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
	Log("Evaluating command: %s inputs:%q", output, rule.inputs)
	for _, input := range rule.inputs {
		if len(rule.outputPatterns) > 0 {
			if len(rule.outputPatterns) > 1 {
				panic("TODO: multiple output pattern is not supported yet")
			}
			input = rule.outputPatterns[0].subst(input, output)
		} else if rule.isSuffixRule {
			input = replaceSuffix(output, input)
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

func (db *DepBuilder) populateSuffixRule(rule *Rule, output string) bool {
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
	r := &Rule{}
	*r = *rule
	r.inputs = []string{inputSuffix}
	r.isSuffixRule = true
	db.suffixRules[outputSuffix] = append([]*Rule{r}, db.suffixRules[outputSuffix]...)
	return true
}

func mergeRules(oldRule, rule *Rule, output string, isSuffixRule bool) *Rule {
	if oldRule.isDoubleColon != rule.isDoubleColon {
		Error(rule.filename, rule.lineno, "*** target file %q has both : and :: entries.", output)
	}
	if len(oldRule.cmds) > 0 && len(rule.cmds) > 0 && !isSuffixRule && !rule.isDoubleColon {
		Warn(rule.filename, rule.cmdLineno, "overriding commands for target %q", output)
		Warn(oldRule.filename, oldRule.cmdLineno, "ignoring old commands for target %q", output)
	}

	r := &Rule{}
	*r = *rule
	if rule.isDoubleColon {
		r.cmds = append(oldRule.cmds, r.cmds...)
	} else if len(oldRule.cmds) > 0 && len(rule.cmds) == 0 {
		r.cmds = oldRule.cmds
	}
	// If the latter rule has a command (regardless of the
	// commands in oldRule), inputs in the latter rule has a
	// priority.
	if len(rule.cmds) > 0 {
		r.inputs = append(r.inputs, oldRule.inputs...)
		r.orderOnlyInputs = append(r.orderOnlyInputs, oldRule.orderOnlyInputs...)
	} else {
		r.inputs = append(oldRule.inputs, r.inputs...)
		r.orderOnlyInputs = append(oldRule.orderOnlyInputs, r.orderOnlyInputs...)
	}
	r.outputPatterns = append(r.outputPatterns, oldRule.outputPatterns...)
	return r
}

func (db *DepBuilder) populateExplicitRule(rule *Rule) {
	// It seems rules with no outputs are siliently ignored.
	if len(rule.outputs) == 0 {
		return
	}
	for _, output := range rule.outputs {
		output = trimLeadingCurdir(output)

		isSuffixRule := db.populateSuffixRule(rule, output)

		if oldRule, present := db.rules[output]; present {
			r := mergeRules(oldRule, rule, output, isSuffixRule)
			db.rules[output] = r
		} else {
			db.rules[output] = rule
			if db.firstRule == nil && !strings.HasPrefix(output, ".") {
				db.firstRule = rule
			}
		}
	}
}

func (db *DepBuilder) populateImplicitRule(irules []*Rule, rule *Rule) []*Rule {
	for _, outputPattern := range rule.outputPatterns {
		r := &Rule{}
		*r = *rule
		r.outputPatterns = []pattern{outputPattern}
		irules = append(irules, r)
	}
	return irules
}

func (db *DepBuilder) populateRules(er *EvalResult) {
	var implicitRules []*Rule
	for _, rule := range er.rules {
		for i, input := range rule.inputs {
			rule.inputs[i] = trimLeadingCurdir(input)
		}
		for i, orderOnlyInput := range rule.orderOnlyInputs {
			rule.orderOnlyInputs[i] = trimLeadingCurdir(orderOnlyInput)
		}
		db.populateExplicitRule(rule)

		if len(rule.outputs) == 0 {
			implicitRules = db.populateImplicitRule(implicitRules, rule)
		}
	}

	// Reverse the implicit rule for easier lookup.
	for i, r := range implicitRules {
		if i >= len(implicitRules)/2 {
			break
		}
		j := len(implicitRules) - i - 1
		implicitRules[i] = implicitRules[j]
		implicitRules[j] = r
	}
	db.nImplicitRules = len(implicitRules)

	var outputs []string
	for output := range db.rules {
		outputs = append(outputs, output)
	}
	sort.Strings(outputs)
	for _, irule := range implicitRules {
		pat := irule.outputPatterns[0]
		if pat.prefix == "" && pat.suffix == "" {
			// TODO(ukai): in android, it is used for turning off
			// RCS / SCCS implicit rules of GNU make, so we could
			// ignore such implicit rules(?)
			for _, output := range outputs {
				db.irules[output] = append(db.irules[output], irule)
			}
			continue
		}
		if pat.prefix != "" {
			i := sort.SearchStrings(outputs, pat.prefix)
			for i < len(outputs) && strings.HasPrefix(outputs[i], pat.prefix) {
				if strings.HasSuffix(outputs[i], pat.suffix) {
					db.irules[outputs[i]] = append(db.irules[outputs[i]], irule)
				}
				i++
			}
			continue
		}
		if pat.suffix != "" {
			// TODO(ukai): handle this. no such rule in android build?
			panic(fmt.Sprintf("suffix rule found: %s at %s:%d", pat, irule.filename, irule.lineno))
		}
	}
}

func (db *DepBuilder) reportStats() {
	if !katiLogFlag && !katiStatsFlag {
		return
	}

	LogStats("node=%d explicit=%d implicit=%d suffix=%d explicitWOCmd=%d",
		db.nodeCnt, db.pickExplicitRuleCnt, db.pickImplicitRuleCnt, db.pickSuffixRuleCnt, db.pickExplicitRuleWithoutCmdCnt)
	if len(db.trace) > 1 {
		LogStats("trace=%q", db.trace)
	}
}

func NewDepBuilder(er *EvalResult, vars Vars) *DepBuilder {
	db := &DepBuilder{
		rules:       make(map[string]*Rule),
		ruleVars:    er.ruleVars,
		irules:      make(map[string][]*Rule),
		suffixRules: make(map[string][]*Rule),
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

func (db *DepBuilder) Eval(targets []string) ([]*DepNode, error) {
	if len(targets) == 0 {
		if db.firstRule == nil {
			ErrorNoLocation("*** No targets.")
		}
		targets = append(targets, db.firstRule.outputs[0])
	}

	LogStats("%d variables", len(db.vars))
	LogStats("%d explicit rules", len(db.rules))
	LogStats("%d implicit rules", db.nImplicitRules)
	LogStats("%d suffix rules", len(db.suffixRules))

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
