package main

import (
	"fmt"
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
}

func newExecutor() *Executor {
	return &Executor{
		rules:       make(map[string]*Rule),
		suffixRules: make(map[string][]*Rule),
	}
}

// TODO(ukai): use time.Time?
func getTimestamp(filename string) int64 {
	st, err := os.Stat(filename)
	if err != nil {
		return -2
	}
	return st.ModTime().Unix()
}

func (ex *Executor) runCommands(cmds []string, output string) error {
Loop:
	for _, cmd := range cmds {
		echo := true
		ignoreErr := false
		for {
			if cmd == "" {
				continue Loop
			}
			switch cmd[0] {
			case '@':
				echo = false
				cmd = cmd[1:]
				continue
			case '-':
				ignoreErr = true
				cmd = cmd[1:]
				continue
			}
			break
		}
		if echo {
			fmt.Printf("%s\n", cmd)
		}
		if dryRunFlag {
			continue
		}

		args := []string{"/bin/sh", "-c", cmd}
		cmd := exec.Cmd{
			Path: args[0],
			Args: args,
		}
		out, err := cmd.CombinedOutput()
		exit := 0
		if err != nil {
			exit = 1
			if err, ok := err.(*exec.ExitError); ok {
				if w, ok := err.ProcessState.Sys().(syscall.WaitStatus); ok {
					exit = w.ExitStatus()
				}
			} else {
				return err
			}
		}
		fmt.Printf("%s", out)
		if exit != 0 {
			if ignoreErr {
				fmt.Printf("[%s] Error %d (ignored)\n", output, exit)
				continue
			}
			return fmt.Errorf("command failed: %q. Error %d", cmd, exit)
		}
	}
	return nil
}

func replaceSuffix(s string, newsuf string) string {
	// TODO: Factor out the logic around suffix rules and use
	// it from substitution references.
	// http://www.gnu.org/software/make/manual/make.html#Substitution-Refs
	oldsuf := filepath.Ext(s)
	return fmt.Sprintf("%s.%s", s[:len(s)-len(oldsuf)], newsuf)
}

func (ex *Executor) canPickImplicitRule(rule *Rule, output string) bool {
	outputPattern := rule.outputPatterns[0]
	if !matchPattern(outputPattern, output) {
		return false
	}
	for _, input := range rule.inputs {
		input = substPattern(outputPattern, input, output)
		if !exists(input) {
			return false
		}
	}
	return true
}

func (ex *Executor) pickRule(output string) (*Rule, bool) {
	rule, present := ex.rules[output]
	if present {
		return rule, true
	}

	for _, rule := range ex.implicitRules {
		if ex.canPickImplicitRule(rule, output) {
			return rule, true
		}
	}

	outputSuffix := filepath.Ext(output)
	if len(outputSuffix) > 0 && outputSuffix[0] == '.' {
		rules, present := ex.suffixRules[outputSuffix[1:]]
		if present {
			for _, rule := range rules {
				if len(rule.inputs) != 1 {
					panic(fmt.Sprintf("unexpected number of input for a suffix rule (%d)", len(rule.inputs)))
				}
				if exists(replaceSuffix(output, rule.inputs[0])) {
					return rule, true
				}
			}
		}
	}

	return nil, false
}

func (ex *Executor) build(vars *VarTab, output string) (int64, error) {
	Log("Building: %s", output)
	outputTs := getTimestamp(output)

	rule, present := ex.pickRule(output)
	if !present {
		if outputTs >= 0 {
			return outputTs, nil
		}
		return outputTs, fmt.Errorf("no rule to make target %q", output)
	}

	latest := int64(-1)
	var actualInputs []string
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

		ts, err := ex.build(vars, input)
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
		ts, err := ex.build(vars, input)
		if err != nil {
			return outputTs, err
		}
		if latest < ts {
			latest = ts
		}
	}

	if outputTs >= latest {
		return outputTs, nil
	}

	localVars := NewVarTab(vars)
	// automatic variables.
	localVars.Assign("@", SimpleVar{value: output, origin: "automatic"})
	if len(actualInputs) > 0 {
		localVars.Assign("<", SimpleVar{
			value:  actualInputs[0],
			origin: "automatic",
		})
		localVars.Assign("^", SimpleVar{
			value:  strings.Join(actualInputs, " "),
			origin: "automatic",
		})
	}
	ev := newEvaluator(localVars)
	var cmds []string
	for _, cmd := range rule.cmds {
		if strings.IndexByte(cmd, '$') < 0 {
			// fast path.
			cmds = append(cmds, cmd)
			continue
		}
		ecmd := ev.evalExpr(cmd)
		Log("build eval:%q => %q", cmd, ecmd)
		cmds = append(cmds, strings.Split(ecmd, "\n")...)
	}

	err := ex.runCommands(cmds, output)
	if err != nil {
		return outputTs, err
	}

	outputTs = getTimestamp(output)
	if outputTs < 0 {
		outputTs = time.Now().Unix()
	}
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

func (ex *Executor) populateExplicitRule(rule *Rule) {
	for _, output := range rule.outputs {
		isSuffixRule := ex.populateSuffixRule(rule, output)

		if oldRule, present := ex.rules[output]; present {
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
			}
			r.inputs = append(r.inputs, oldRule.inputs...)
			ex.rules[output] = r
		} else {
			ex.rules[output] = rule
			if ex.firstRule == nil && !isSuffixRule {
				ex.firstRule = rule
			}
		}
	}
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

func (ex *Executor) exec(er *EvalResult, targets []string) error {
	ex.populateRules(er)

	if len(targets) == 0 {
		if ex.firstRule == nil {
			ErrorNoLocation("*** No targets.")
		}
		targets = append(targets, ex.firstRule.outputs[0])
	}

	for _, target := range targets {
		_, err := ex.build(er.vars, target)
		if err != nil {
			return err
		}
	}
	return nil
}

func Exec(er *EvalResult, targets []string) error {
	ex := newExecutor()
	return ex.exec(er, targets)
}
