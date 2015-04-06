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

func (ex *Executor) runCommands(cmds []string, output string) error {
Loop:
	for _, cmd := range cmds {
		echo := true
		ignoreErr := false
		for {
			cmd = strings.TrimLeft(cmd, " \t")
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
		if len(rule.cmds) > 0 {
			return rule, true
		}
		// If none of the explicit rules for a target has commands,
		// then `make' searches for an applicable implicit rule to
		// find some commands.
	}

	for _, irule := range ex.implicitRules {
		if !ex.canPickImplicitRule(irule, output) {
			continue
		}
		if rule != nil {
			r := &Rule{}
			*r = *rule
			r.outputPatterns = irule.outputPatterns
			// implicit rule's prerequisites will be used for $<
			r.inputs = append(irule.inputs, r.inputs...)
			if irule.vars != nil {
				r.vars = NewVarTab(rule.vars)
				for k, v := range irule.vars.m {
					r.vars.Assign(k, v)
				}
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
		if rule != nil {
			r := &Rule{}
			*r = *rule
			// TODO(ukai): input order is correct?
			r.inputs = append([]string{replaceSuffix(output, irule.inputs[0])}, r.inputs...)
			r.vars = NewVarTab(rule.vars)
			if irule.vars != nil {
				for k, v := range irule.vars.m {
					r.vars.Assign(k, v)
				}
			}
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
	if rule.vars != nil {
		vars = NewVarTab(vars)
		for k, v := range rule.vars.m {
			vars.Assign(k, v)
		}
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
	ev.filename = rule.filename
	ev.lineno = rule.cmdLineno
	var cmds []string
	Log("Building: %s cmds:%q", output, rule.cmds)
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
	// It seems rules with no outputs are siliently ignored.
	if len(rule.outputs) == 0 {
		return
	}
	for _, output := range rule.outputs {

		isSuffixRule := ex.populateSuffixRule(rule, output)

		if oldRule, present := ex.rules[output]; present {
			if oldRule.vars != nil || rule.vars != nil {
				oldRule.isDoubleColon = rule.isDoubleColon
				switch {
				case rule.vars == nil && oldRule.vars != nil:
					rule.vars = oldRule.vars
				case rule.vars != nil && oldRule.vars == nil:
				case rule.vars != nil && oldRule.vars != nil:
					// parent would be the same vars?
					for k, v := range rule.vars.m {
						oldRule.vars.m[k] = v
					}
					rule.vars = oldRule.vars
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

func (ex *Executor) exec(er *EvalResult, targets []string, vars *VarTab) error {
	ex.populateRules(er)

	if len(targets) == 0 {
		if ex.firstRule == nil {
			ErrorNoLocation("*** No targets.")
		}
		targets = append(targets, ex.firstRule.outputs[0])
	}

	for _, target := range targets {
		_, err := ex.build(vars, target)
		if err != nil {
			return err
		}
	}
	return nil
}

func Exec(er *EvalResult, targets []string, vars *VarTab) error {
	ex := newExecutor()
	return ex.exec(er, targets, vars)
}
