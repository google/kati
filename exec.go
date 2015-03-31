package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	for _, cmd := range cmds {
		cmd = expandCommandVars(cmd, output)
		fmt.Printf("%s\n", cmd)

		args := []string{"/bin/sh", "-c", cmd}
		cmd := exec.Cmd{
			Path: args[0],
			Args: args,
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			return err
		}
		success := false
		if cmd.ProcessState != nil {
			success = cmd.ProcessState.Success()
		}

		fmt.Printf("%s", out)
		if !success {
			return fmt.Errorf("command failed: %q", cmd)
		}
	}
	return nil
}

func (ex *Executor) build(output string) (int64, error) {
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
		ts, err := ex.build(input)
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

	err := ex.runCommands(rule.cmds, output)
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
		return errors.New("no targets.")
	}

	for _, rule := range er.rules {
		if oldRule, present := ex.rules[rule.output]; present {
			if len(oldRule.cmds) > 0 && len(rule.cmds) > 0 {
				Warn(rule.filename, rule.cmdLineno, "overriding commands for target %q", rule.output)
				Warn(oldRule.filename, oldRule.cmdLineno, "ignoring old commands for target %q", oldRule.output)
			}
			r := &Rule{}
			*r = *rule
			r.inputs = append(r.inputs, oldRule.inputs...)
			ex.rules[rule.output] = r
		} else {
			ex.rules[rule.output] = rule
		}
	}

	if len(targets) == 0 {
		targets = append(targets, er.rules[0].output)
	}

	for _, target := range targets {
		_, err := ex.build(target)
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
