package main

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

type Runner struct {
	cmd         string
	echo        bool
	ignoreError bool
	shell       string
}

type DepNode struct {
	Output      string
	Runners     []Runner
	Deps        []*DepNode
	HasRule     bool
	IsOrderOnly bool
}

func (n *DepNode) String() string {
	return fmt.Sprintf("Dep{output=%s runners=%d deps=%d hasRule=%t orderOnly=%t}",
		n.Output, len(n.Runners), len(n.Deps), n.HasRule, n.IsOrderOnly)
}

type CommandEvaluator struct {
	rules         map[string]*Rule
	ruleVars      map[string]Vars
	implicitRules []*Rule
	suffixRules   map[string][]*Rule
	firstRule     *Rule
	shell         string
	vars          Vars
	done          map[string]*DepNode

	currentOutput string
	currentInputs []string
	currentStem   string

	trace                         []string
	nodeCnt                       int
	pickExplicitRuleCnt           int
	pickImplicitRuleCnt           int
	pickSuffixRuleCnt             int
	pickExplicitRuleWithoutCmdCnt int
}

type AutoVar struct{ ce *CommandEvaluator }

func (v AutoVar) Flavor() string  { return "undefined" }
func (v AutoVar) Origin() string  { return "automatic" }
func (v AutoVar) IsDefined() bool { panic("not implemented") }
func (v AutoVar) String() string  { panic("not implemented") }
func (v AutoVar) Append(*Evaluator, string) Var {
	panic("must not be called")
}
func (v AutoVar) AppendVar(*Evaluator, Var) Var {
	panic("must not be called")
}

type AutoAtVar struct{ AutoVar }

func (v AutoAtVar) Eval(w io.Writer, ev *Evaluator) {
	fmt.Fprint(w, v.ce.currentOutput)
}

type AutoLessVar struct{ AutoVar }

func (v AutoLessVar) Eval(w io.Writer, ev *Evaluator) {
	if len(v.ce.currentInputs) > 0 {
		fmt.Fprint(w, v.ce.currentInputs[0])
	}
}

type AutoHatVar struct{ AutoVar }

func (v AutoHatVar) Eval(w io.Writer, ev *Evaluator) {
	var uniqueInputs []string
	seen := make(map[string]bool)
	for _, input := range v.ce.currentInputs {
		if !seen[input] {
			seen[input] = true
			uniqueInputs = append(uniqueInputs, input)
		}
	}
	fmt.Fprint(w, strings.Join(uniqueInputs, " "))
}

type AutoPlusVar struct{ AutoVar }

func (v AutoPlusVar) Eval(w io.Writer, ev *Evaluator) {
	fmt.Fprint(w, strings.Join(v.ce.currentInputs, " "))
}

type AutoStarVar struct{ AutoVar }

func (v AutoStarVar) Eval(w io.Writer, ev *Evaluator) {
	// TODO: Use currentStem. See auto_stem_var.mk
	fmt.Fprint(w, stripExt(v.ce.currentOutput))
}

type AutoSuffixDVar struct {
	AutoVar
	v Var
}

func (v AutoSuffixDVar) Eval(w io.Writer, ev *Evaluator) {
	var buf bytes.Buffer
	v.v.Eval(&buf, ev)
	for i, tok := range splitSpaces(buf.String()) {
		if i > 0 {
			w.Write([]byte{' '})
		}
		fmt.Fprint(w, filepath.Dir(tok))
	}
}

type AutoSuffixFVar struct {
	AutoVar
	v Var
}

func (v AutoSuffixFVar) Eval(w io.Writer, ev *Evaluator) {
	var buf bytes.Buffer
	v.v.Eval(&buf, ev)
	for i, tok := range splitSpaces(buf.String()) {
		if i > 0 {
			w.Write([]byte{' '})
		}
		fmt.Fprint(w, filepath.Base(tok))
	}
}

func replaceSuffix(s string, newsuf string) string {
	// TODO: Factor out the logic around suffix rules and use
	// it from substitution references.
	// http://www.gnu.org/software/make/manual/make.html#Substitution-Refs
	return fmt.Sprintf("%s.%s", stripExt(s), newsuf)
}

func (ce *CommandEvaluator) exists(target string) bool {
	_, present := ce.rules[target]
	if present {
		return true
	}
	rule, present := ce.rules[".PHONY"]
	if present {
		for _, input := range rule.inputs {
			if target == input {
				return true
			}
		}
	}
	return exists(target)
}

func (ce *CommandEvaluator) canPickImplicitRule(rule *Rule, output string) bool {
	outputPattern := rule.outputPatterns[0]
	if !matchPattern(outputPattern, output) {
		return false
	}
	for _, input := range rule.inputs {
		input = substPattern(outputPattern, input, output)
		if !ce.exists(input) {
			return false
		}
	}
	return true
}

func (ce *CommandEvaluator) mergeImplicitRuleVars(outputs []string, vars Vars) Vars {
	if len(outputs) != 1 {
		panic(fmt.Sprintf("Implicit rule should have only one output but %q", outputs))
	}
	Log("merge? %q", ce.ruleVars)
	Log("merge? %q", outputs[0])
	ivars, present := ce.ruleVars[outputs[0]]
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

func (ce *CommandEvaluator) pickRule(output string) (*Rule, Vars, bool) {
	rule, present := ce.rules[output]
	vars := ce.ruleVars[output]
	if present {
		ce.pickExplicitRuleCnt++
		if len(rule.cmds) > 0 {
			return rule, vars, true
		}
		// If none of the explicit rules for a target has commands,
		// then `make' searches for an applicable implicit rule to
		// find some commands.
		ce.pickExplicitRuleWithoutCmdCnt++
	}

	for _, irule := range ce.implicitRules {
		if !ce.canPickImplicitRule(irule, output) {
			continue
		}
		ce.pickImplicitRuleCnt++
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
			vars = ce.mergeImplicitRuleVars(irule.outputPatterns, vars)
		}
		// TODO(ukai): check len(irule.cmd) ?
		return irule, vars, true
	}

	outputSuffix := filepath.Ext(output)
	if !strings.HasPrefix(outputSuffix, ".") {
		return rule, vars, rule != nil
	}
	rules, present := ce.suffixRules[outputSuffix[1:]]
	if !present {
		return rule, vars, rule != nil
	}
	for _, irule := range rules {
		if len(irule.inputs) != 1 {
			panic(fmt.Sprintf("unexpected number of input for a suffix rule (%d)", len(irule.inputs)))
		}
		if !ce.exists(replaceSuffix(output, irule.inputs[0])) {
			continue
		}
		ce.pickSuffixRuleCnt++
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
			vars = ce.mergeImplicitRuleVars(irule.outputs, vars)
		}
		// TODO(ukai): check len(irule.cmd) ?
		return irule, vars, true
	}
	return rule, vars, rule != nil
}

func newRunner(r Runner, s string) Runner {
	for {
		s = trimLeftSpace(s)
		if s == "" {
			return Runner{}
		}
		switch s[0] {
		case '@':
			r.echo = false
			s = s[1:]
			continue
		case '-':
			r.ignoreError = true
			s = s[1:]
			continue
		}
		break
	}
	r.cmd = s
	return r
}

func evalCmd(ev *Evaluator, r Runner, s string) []Runner {
	r = newRunner(r, s)
	if strings.IndexByte(r.cmd, '$') < 0 {
		// fast path
		return []Runner{r}
	}
	cmds := ev.evalExpr(r.cmd)
	var runners []Runner
	for _, cmd := range strings.Split(cmds, "\n") {
		if len(runners) > 0 && strings.HasSuffix(runners[0].cmd, "\\") {
			runners[0].cmd += "\n"
			runners[0].cmd += cmd
		} else {
			runners = append(runners, newRunner(r, cmd))
		}
	}
	return runners
}

func (ce *CommandEvaluator) buildPlan(output string, neededBy string) (*DepNode, error) {
	Log("Evaluating command: %s", output)
	ce.nodeCnt++
	if ce.nodeCnt%100 == 0 {
		ce.reportStats()
	}

	if n, present := ce.done[output]; present {
		return n, nil
	}
	n := &DepNode{Output: output}
	ce.done[output] = n

	rule, vars, present := ce.pickRule(output)
	if !present {
		return n, nil
	}

	var restores []func()
	if vars != nil {
		for name, v := range vars {
			tsv := v.(TargetSpecificVar)
			restores = append(restores, ce.vars.save(name))
			switch tsv.op {
			case ":=", "=":
				ce.vars[name] = tsv
			case "+=":
				oldVar, present := ce.vars[name]
				if !present || oldVar.String() == "" {
					ce.vars[name] = tsv
				} else {
					ce.vars[name] = oldVar.AppendVar(newEvaluator(ce.vars), tsv)
				}
			case "?=":
				if _, present := ce.vars[name]; !present {
					ce.vars[name] = tsv
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
			input = substPattern(rule.outputPatterns[0], input, output)
		} else if rule.isSuffixRule {
			input = replaceSuffix(output, input)
		}
		actualInputs = append(actualInputs, input)

		ce.trace = append(ce.trace, input)
		n, err := ce.buildPlan(input, output)
		ce.trace = ce.trace[0 : len(ce.trace)-1]
		if err != nil {
			return nil, err
		}
		if n != nil {
			children = append(children, n)
		}
	}

	for _, input := range rule.orderOnlyInputs {
		ce.trace = append(ce.trace, input)
		n, err := ce.buildPlan(input, output)
		ce.trace = ce.trace[0 : len(ce.trace)-1]
		if err != nil {
			return nil, err
		}
		if n != nil {
			n.IsOrderOnly = true
			children = append(children, n)
		}
	}

	// For automatic variables.
	ce.currentOutput = output
	ce.currentInputs = actualInputs

	ev := newEvaluator(ce.vars)
	ev.filename = rule.filename
	ev.lineno = rule.cmdLineno
	var runners []Runner
	Log("Evaluating command: %s cmds:%q", output, rule.cmds)
	r := Runner{
		echo:  true,
		shell: ce.shell,
	}
	for _, cmd := range rule.cmds {
		for _, r := range evalCmd(ev, r, cmd) {
			if len(r.cmd) != 0 {
				runners = append(runners, r)
			}
		}
	}

	n.Runners = runners
	n.Deps = children
	n.HasRule = true
	return n, nil
}

func (ce *CommandEvaluator) populateSuffixRule(rule *Rule, output string) bool {
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
	ce.suffixRules[outputSuffix] = append([]*Rule{r}, ce.suffixRules[outputSuffix]...)
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

func (ce *CommandEvaluator) populateExplicitRule(rule *Rule) {
	// It seems rules with no outputs are siliently ignored.
	if len(rule.outputs) == 0 {
		return
	}
	for _, output := range rule.outputs {
		output = filepath.Clean(output)

		isSuffixRule := ce.populateSuffixRule(rule, output)

		if oldRule, present := ce.rules[output]; present {
			r := mergeRules(oldRule, rule, output, isSuffixRule)
			ce.rules[output] = r
		} else {
			ce.rules[output] = rule
			if ce.firstRule == nil && !strings.HasPrefix(output, ".") {
				ce.firstRule = rule
			}
		}
	}
}

func (ce *CommandEvaluator) populateImplicitRule(rule *Rule) {
	for _, outputPattern := range rule.outputPatterns {
		r := &Rule{}
		*r = *rule
		r.outputPatterns = []string{outputPattern}
		ce.implicitRules = append(ce.implicitRules, r)
	}
}

func (ce *CommandEvaluator) populateRules(er *EvalResult) {
	for _, rule := range er.rules {
		for i, input := range rule.inputs {
			rule.inputs[i] = filepath.Clean(input)
		}
		for i, orderOnlyInput := range rule.orderOnlyInputs {
			rule.orderOnlyInputs[i] = filepath.Clean(orderOnlyInput)
		}
		ce.populateExplicitRule(rule)

		if len(rule.outputs) == 0 {
			ce.populateImplicitRule(rule)
		}
	}

	// Reverse the implicit rule for easier lookup.
	for i, r := range ce.implicitRules {
		if i >= len(ce.implicitRules)/2 {
			break
		}
		j := len(ce.implicitRules) - i - 1
		ce.implicitRules[i] = ce.implicitRules[j]
		ce.implicitRules[j] = r
	}
}

func (ce *CommandEvaluator) reportStats() {
	if !katiLogFlag && !katiStatsFlag {
		return
	}

	LogStats("node=%d explicit=%d implicit=%d suffix=%d explicitWOCmd=%d",
		ce.nodeCnt, ce.pickExplicitRuleCnt, ce.pickImplicitRuleCnt, ce.pickSuffixRuleCnt, ce.pickExplicitRuleWithoutCmdCnt)
	if len(ce.trace) > 1 {
		LogStats("trace=%q", ce.trace)
	}
}

func NewCommandEvaluator(er *EvalResult, vars Vars) *CommandEvaluator {
	ce := &CommandEvaluator{
		rules:       make(map[string]*Rule),
		ruleVars:    er.ruleVars,
		suffixRules: make(map[string][]*Rule),
		vars:        vars,
		done:        make(map[string]*DepNode),
	}

	for k, v := range map[string]Var{
		"@": AutoAtVar{AutoVar: AutoVar{ce: ce}},
		"<": AutoLessVar{AutoVar: AutoVar{ce: ce}},
		"^": AutoHatVar{AutoVar: AutoVar{ce: ce}},
		"+": AutoPlusVar{AutoVar: AutoVar{ce: ce}},
		"*": AutoStarVar{AutoVar: AutoVar{ce: ce}},
	} {
		ce.vars[k] = v
		ce.vars[k+"D"] = AutoSuffixDVar{v: v}
		ce.vars[k+"F"] = AutoSuffixFVar{v: v}
	}

	// TODO: We should move this to somewhere around evalCmd so that
	// we can handle SHELL in target specific variables.
	shellVar := ce.vars.Lookup("SHELL")
	ce.shell = shellVar.String()

	ce.populateRules(er)
	return ce
}

func (ce *CommandEvaluator) Eval(targets []string) ([]*DepNode, error) {
	if len(targets) == 0 {
		if ce.firstRule == nil {
			ErrorNoLocation("*** No targets.")
		}
		targets = append(targets, ce.firstRule.outputs[0])
	}

	LogStats("%d variables", len(ce.vars))
	LogStats("%d explicit rules", len(ce.rules))
	LogStats("%d implicit rules", len(ce.implicitRules))
	LogStats("%d suffix rules", len(ce.suffixRules))

	var nodes []*DepNode
	for _, target := range targets {
		ce.trace = []string{target}
		n, err := ce.buildPlan(target, "")
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	ce.reportStats()
	return nodes, nil
}
