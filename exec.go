package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

type Executor struct {
	rules map[string]*Rule
}

func newExecutor() *Executor {
	return &Executor{
		rules: make(map[string]*Rule),
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

func escapeVar(v string) string {
	return strings.Replace(v, "$", "$$", -1)
}

func (ex *Executor) build(vars map[string]string, output string) (int64, error) {
	Log("Building: %s", output)
	outputTs := getTimestamp(output)

	rule, present := ex.rules[output]
	if !present {
		if outputTs >= 0 {
			return outputTs, nil
		}
		return outputTs, fmt.Errorf("no rule to make target %q", output)
	}

	latest := int64(-1)
	for _, input := range rule.inputs {
		if len(rule.outputPatterns) > 0 {
			if len(rule.outputPatterns) > 1 {
				panic("TODO: multiple output pattern is not supported yet")
			}
			input = substPattern(rule.outputPatterns[0], input, output)
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

	localVars := make(map[string]string)
	for k, v := range vars {
		localVars[k] = v
	}
	// automatic variables.
	localVars["@"] = escapeVar(output)
	Log("local vars: %q", localVars)
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

func (ex *Executor) exec(er *EvalResult, targets []string) error {
	if len(er.rules) == 0 {
		ErrorNoLocation("*** No targets.")
	}

	for _, rule := range er.rules {
		for _, output := range rule.outputs {
			if oldRule, present := ex.rules[output]; present {
				if len(oldRule.cmds) > 0 && len(rule.cmds) > 0 {
					Warn(rule.filename, rule.cmdLineno, "overriding commands for target %q", output)
					Warn(oldRule.filename, oldRule.cmdLineno, "ignoring old commands for target %q", output)
				}
				r := &Rule{}
				*r = *rule
				r.inputs = append(r.inputs, oldRule.inputs...)
				ex.rules[output] = r
			} else {
				ex.rules[output] = rule
			}
		}
	}

	if len(targets) == 0 {
		targets = append(targets, er.rules[0].outputs[0])
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
