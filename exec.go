package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type Executor struct {
	rules         map[string]*Rule
	implicitRules []*Rule
	suffixRules   map[string][]*Rule
	firstRule     *Rule
	shell         string
	vars          Vars
	// target -> timestamp, a negative timestamp means the target is
	// currently being processed.
	done map[string]int64

	currentOutput string
	currentInputs []string
	currentStem   string

	trace                         []string
	buildCnt                      int
	alreadyDoneCnt                int
	noRuleCnt                     int
	upToDateCnt                   int
	runCommandCnt                 int
	pickExplicitRuleCnt           int
	pickImplicitRuleCnt           int
	pickSuffixRuleCnt             int
	pickExplicitRuleWithoutCmdCnt int
}

type AutoVar struct{ ex *Executor }

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
	fmt.Fprint(w, v.ex.currentOutput)
}

type AutoLessVar struct{ AutoVar }

func (v AutoLessVar) Eval(w io.Writer, ev *Evaluator) {
	if len(v.ex.currentInputs) > 0 {
		fmt.Fprint(w, v.ex.currentInputs[0])
	}
}

type AutoHatVar struct{ AutoVar }

func (v AutoHatVar) Eval(w io.Writer, ev *Evaluator) {
	var uniqueInputs []string
	seen := make(map[string]bool)
	for _, input := range v.ex.currentInputs {
		if !seen[input] {
			seen[input] = true
			uniqueInputs = append(uniqueInputs, input)
		}
	}
	fmt.Fprint(w, strings.Join(uniqueInputs, " "))
}

type AutoPlusVar struct{ AutoVar }

func (v AutoPlusVar) Eval(w io.Writer, ev *Evaluator) {
	fmt.Fprint(w, strings.Join(v.ex.currentInputs, " "))
}

type AutoStarVar struct{ AutoVar }

func (v AutoStarVar) Eval(w io.Writer, ev *Evaluator) {
	// TODO: Use currentStem. See auto_stem_var.mk
	fmt.Fprint(w, stripExt(v.ex.currentOutput))
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

func newExecutor(vars Vars) *Executor {
	ex := &Executor{
		rules:       make(map[string]*Rule),
		suffixRules: make(map[string][]*Rule),
		done:        make(map[string]int64),
		vars:        vars,
	}

	for k, v := range map[string]Var{
		"@": AutoAtVar{AutoVar: AutoVar{ex: ex}},
		"<": AutoLessVar{AutoVar: AutoVar{ex: ex}},
		"^": AutoHatVar{AutoVar: AutoVar{ex: ex}},
		"+": AutoPlusVar{AutoVar: AutoVar{ex: ex}},
		"*": AutoStarVar{AutoVar: AutoVar{ex: ex}},
	} {
		ex.vars[k] = v
		ex.vars[k+"D"] = AutoSuffixDVar{v: v}
		ex.vars[k+"F"] = AutoSuffixFVar{v: v}
	}

	return ex
}

// TODO(ukai): use time.Time?
func getTimestamp(filename string) int64 {
	st, err := os.Stat(filename)
	if err != nil {
		return -2
	}
	return st.ModTime().Unix()
}

func (ex *Executor) exists(target string) bool {
	_, present := ex.rules[target]
	if present {
		return true
	}
	rule, present := ex.rules[".PHONY"]
	if present {
		for _, input := range rule.inputs {
			if target == input {
				return true
			}
		}
	}
	return exists(target)
}

type runner struct {
	output      string
	cmd         string
	echo        bool
	dryRun      bool
	ignoreError bool
	shell       string
}

func evalCmd(ev *Evaluator, r runner, s string) []runner {
	r = newRunner(r, s)
	if strings.IndexByte(r.cmd, '$') < 0 {
		// fast path
		return []runner{r}
	}
	cmds := ev.evalExpr(r.cmd)
	var runners []runner
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

func newRunner(r runner, s string) runner {
	for {
		s = trimLeftSpace(s)
		if s == "" {
			return runner{}
		}
		switch s[0] {
		case '@':
			if !r.dryRun {
				r.echo = false
			}
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

func (r runner) run() error {
	if r.echo {
		fmt.Printf("%s\n", r.cmd)
	}
	if r.dryRun {
		return nil
	}
	args := []string{r.shell, "-c", r.cmd}
	cmd := exec.Cmd{
		Path: args[0],
		Args: args,
	}
	out, err := cmd.CombinedOutput()
	fmt.Printf("%s", out)
	exit := exitStatus(err)
	if r.ignoreError && exit != 0 {
		fmt.Printf("[%s] Error %d (ignored)\n", r.output, exit)
		err = nil
	}
	return err
}

func exitStatus(err error) int {
	if err == nil {
		return 0
	}
	exit := 1
	if err, ok := err.(*exec.ExitError); ok {
		if w, ok := err.ProcessState.Sys().(syscall.WaitStatus); ok {
			return w.ExitStatus()
		}
	}
	return exit
}

func replaceSuffix(s string, newsuf string) string {
	// TODO: Factor out the logic around suffix rules and use
	// it from substitution references.
	// http://www.gnu.org/software/make/manual/make.html#Substitution-Refs
	return fmt.Sprintf("%s.%s", stripExt(s), newsuf)
}

func (ex *Executor) canPickImplicitRule(rule *Rule, output string) bool {
	outputPattern := rule.outputPatterns[0]
	if !matchPattern(outputPattern, output) {
		return false
	}
	for _, input := range rule.inputs {
		input = substPattern(outputPattern, input, output)
		if !ex.exists(input) {
			return false
		}
	}
	return true
}

func (ex *Executor) pickRule(output string) (*Rule, bool) {
	rule, present := ex.rules[output]
	if present {
		ex.pickExplicitRuleCnt++
		if len(rule.cmds) > 0 {
			return rule, true
		}
		// If none of the explicit rules for a target has commands,
		// then `make' searches for an applicable implicit rule to
		// find some commands.
		ex.pickExplicitRuleWithoutCmdCnt++
	}

	for _, irule := range ex.implicitRules {
		if !ex.canPickImplicitRule(irule, output) {
			continue
		}
		ex.pickImplicitRuleCnt++
		if rule != nil {
			r := &Rule{}
			*r = *rule
			r.outputPatterns = irule.outputPatterns
			// implicit rule's prerequisites will be used for $<
			r.inputs = append(irule.inputs, r.inputs...)
			if irule.vars != nil {
				r.vars = append(r.vars, rule.vars...)
				r.vars = append(r.vars, irule.vars...)
			}
			r.cmds = irule.cmds
			// TODO(ukai): filename, lineno?
			r.cmdLineno = irule.cmdLineno
			return r, true
		}
		// TODO(ukai): check len(irule.cmd) ?
		return irule, true
	}

	outputSuffix := filepath.Ext(output)
	if !strings.HasPrefix(outputSuffix, ".") {
		return rule, rule != nil
	}
	rules, present := ex.suffixRules[outputSuffix[1:]]
	if !present {
		return rule, rule != nil
	}
	for _, irule := range rules {
		if len(irule.inputs) != 1 {
			panic(fmt.Sprintf("unexpected number of input for a suffix rule (%d)", len(irule.inputs)))
		}
		if !ex.exists(replaceSuffix(output, irule.inputs[0])) {
			continue
		}
		ex.pickSuffixRuleCnt++
		if rule != nil {
			r := &Rule{}
			*r = *rule
			// TODO(ukai): input order is correct?
			r.inputs = append([]string{replaceSuffix(output, irule.inputs[0])}, r.inputs...)
			r.vars = append(r.vars, rule.vars...)
			r.vars = append(r.vars, irule.vars...)
			r.cmds = irule.cmds
			// TODO(ukai): filename, lineno?
			r.cmdLineno = irule.cmdLineno
			return r, true
		}
		// TODO(ukai): check len(irule.cmd) ?
		return irule, true
	}
	return rule, rule != nil
}

func (ex *Executor) build(output string, neededBy string) (int64, error) {
	Log("Building: %s", output)
	ex.buildCnt++
	if ex.buildCnt%100 == 0 {
		ex.reportStats()
	}

	outputTs, ok := ex.done[output]
	if ok {
		if outputTs < 0 {
			fmt.Printf("Circular %s <- %s dependency dropped.\n", neededBy, output)
		}
		Log("Building: %s already done: %d", output, outputTs)
		ex.alreadyDoneCnt++
		return outputTs, nil
	}
	ex.done[output] = -1
	outputTs = getTimestamp(output)

	rule, present := ex.pickRule(output)
	if !present {
		if outputTs >= 0 {
			ex.done[output] = outputTs
			ex.noRuleCnt++
			return outputTs, nil
		}
		if neededBy == "" {
			ErrorNoLocation("*** No rule to make target %q.", output)
		} else {
			ErrorNoLocation("*** No rule to make target %q, needed by %q.", output, neededBy)
		}
		return outputTs, fmt.Errorf("no rule to make target %q", output)
	}

	var olds []oldVar
	if rule.vars != nil {
		for _, v := range rule.vars {
			olds = append(olds, newOldVar(ex.vars, v.name))
			switch v.op {
			case ":=", "=":
				ex.vars[v.name] = v.v
			case "+=":
				oldVar := ex.vars[v.name]
				// Not sure why make does not append a
				// whitespace... See
				// target_specific_var_append.
				if oldVar.String() == "" {
					ex.vars[v.name] = v.v
				} else {
					ex.vars[v.name] = oldVar.AppendVar(newEvaluator(ex.vars), v.v)
				}
			case "?=":
				if _, present := ex.vars[v.name]; !present {
					ex.vars[v.name] = v.v
				}
			}
		}
		defer func() {
			for _, old := range olds {
				old.restore(ex.vars)
			}
		}()
	}

	latest := int64(-1)
	var actualInputs []string
	Log("Building: %s inputs:%q", output, rule.inputs)
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

		ex.trace = append(ex.trace, input)
		ts, err := ex.build(input, output)
		ex.trace = ex.trace[0 : len(ex.trace)-1]
		if err != nil {
			return outputTs, err
		}
		if latest < ts {
			latest = ts
		}
	}

	for _, input := range rule.orderOnlyInputs {
		if exists(input) {
			continue
		}

		ex.trace = append(ex.trace, input)
		ts, err := ex.build(input, output)
		ex.trace = ex.trace[0 : len(ex.trace)-1]
		if err != nil {
			return outputTs, err
		}
		if latest < ts {
			latest = ts
		}
	}

	if outputTs >= latest {
		ex.done[output] = outputTs
		ex.upToDateCnt++
		return outputTs, nil
	}

	// For automatic variables.
	ex.currentOutput = output
	ex.currentInputs = actualInputs

	ev := newEvaluator(ex.vars)
	ev.filename = rule.filename
	ev.lineno = rule.cmdLineno
	var runners []runner
	Log("Building: %s cmds:%q", output, rule.cmds)
	r := runner{
		output: output,
		echo:   true,
		dryRun: dryRunFlag,
		shell:  ex.shell,
	}
	for _, cmd := range rule.cmds {
		for _, r := range evalCmd(ev, r, cmd) {
			if len(r.cmd) != 0 {
				runners = append(runners, r)
			}
		}
	}
	for _, r := range runners {
		err := r.run()
		if err != nil {
			exit := exitStatus(err)
			fmt.Printf("[%s] Error %d: %v\n", r.output, exit, err)
			return outputTs, err
		}
	}

	outputTs = getTimestamp(output)
	if outputTs < 0 {
		outputTs = time.Now().Unix()
	}
	ex.done[output] = outputTs
	Log("Building: %s done %d", output, outputTs)
	ex.runCommandCnt++
	return outputTs, nil
}

func (ex *Executor) populateSuffixRule(rule *Rule, output string) bool {
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
	ex.suffixRules[outputSuffix] = append([]*Rule{r}, ex.suffixRules[outputSuffix]...)
	return true
}

func mergeRules(oldRule, rule *Rule, output string, isSuffixRule bool) *Rule {
	if oldRule == rule {
		panic("Merging a same rule")
	}
	if oldRule.vars != nil || rule.vars != nil {
		oldRule.isDoubleColon = rule.isDoubleColon
		switch {
		case rule.vars == nil && oldRule.vars != nil:
			rule.vars = oldRule.vars
		case rule.vars != nil && oldRule.vars == nil:
		case rule.vars != nil && oldRule.vars != nil:
			// parent would be the same vars?
			rule.vars = append(oldRule.vars, rule.vars...)
		}
	}

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

func (ex *Executor) populateExplicitRule(rule *Rule) {
	fmt.Printf("populateExplicitRule\n")
	// It seems rules with no outputs are siliently ignored.
	if len(rule.outputs) == 0 {
		return
	}
	for _, output := range rule.outputs {
		output = filepath.Clean(output)

		isSuffixRule := ex.populateSuffixRule(rule, output)

		if oldRule, present := ex.rules[output]; present {
			fmt.Printf("merge\n")
			r := mergeRules(oldRule, rule, output, isSuffixRule)
			fmt.Printf("merge done\n")
			ex.rules[output] = r
		} else {
			ex.rules[output] = rule
			if ex.firstRule == nil && !strings.HasPrefix(output, ".") {
				ex.firstRule = rule
			}
		}
	}
	fmt.Printf("populateExplicitRule done\n")
}

func (ex *Executor) populateImplicitRule(rule *Rule) {
	for _, outputPattern := range rule.outputPatterns {
		r := &Rule{}
		*r = *rule
		r.outputPatterns = []string{outputPattern}
		ex.implicitRules = append(ex.implicitRules, r)
	}
}

func (ex *Executor) populateRules(er *EvalResult) {
	for _, rule := range er.rules {
		for i, input := range rule.inputs {
			rule.inputs[i] = filepath.Clean(input)
		}
		for i, orderOnlyInput := range rule.orderOnlyInputs {
			rule.orderOnlyInputs[i] = filepath.Clean(orderOnlyInput)
		}
		ex.populateExplicitRule(rule)

		if len(rule.outputs) == 0 {
			ex.populateImplicitRule(rule)
		}
	}

	// Reverse the implicit rule for easier lookup.
	for i, r := range ex.implicitRules {
		if i >= len(ex.implicitRules)/2 {
			break
		}
		j := len(ex.implicitRules) - i - 1
		ex.implicitRules[i] = ex.implicitRules[j]
		ex.implicitRules[j] = r
	}
}

func (ex *Executor) reportStats() {
	if !katiLogFlag && !katiStatsFlag {
		return
	}

	LogStats("build=%d alreadyDone=%d noRule=%d, upToDate=%d runCommand=%d",
		ex.buildCnt, ex.alreadyDoneCnt, ex.noRuleCnt, ex.upToDateCnt, ex.runCommandCnt)
	LogStats("explicit=%d implicit=%d suffix=%d explicitWOCmd=%d",
		ex.pickExplicitRuleCnt, ex.pickImplicitRuleCnt, ex.pickSuffixRuleCnt, ex.pickExplicitRuleWithoutCmdCnt)
	if len(ex.trace) > 1 {
		LogStats("trace=%q", ex.trace)
	}
}

func NewExecutor(er *EvalResult, vars Vars) *Executor {
	ex := newExecutor(vars)
	// TODO: We should move this to somewhere around evalCmd so that
	// we can handle SHELL in target specific variables.
	shellVar := ex.vars.Lookup("SHELL")
	ex.shell = shellVar.String()

	ex.populateRules(er)
	return ex
}

func (ex *Executor) Exec(targets []string) error {
	if len(targets) == 0 {
		if ex.firstRule == nil {
			ErrorNoLocation("*** No targets.")
		}
		targets = append(targets, ex.firstRule.outputs[0])
	}

	LogStats("%d variables", len(ex.vars))
	LogStats("%d explicit rules", len(ex.rules))
	LogStats("%d implicit rules", len(ex.implicitRules))
	LogStats("%d suffix rules", len(ex.suffixRules))

	for _, target := range targets {
		ex.trace = []string{target}
		_, err := ex.build(target, "")
		if err != nil {
			return err
		}
	}
	ex.reportStats()
	return nil
}
