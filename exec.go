package main

import (
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

func getTimestamp(filename string) int64 {
	st, err := os.Stat(filename)
	if err != nil {
		return -2
	}
	return st.ModTime().Unix()
}

func (ex *Executor) runCommands(cmds []string) {
	for _, cmd := range cmds {
		fmt.Printf("%s\n", cmd)

		args := []string{"/bin/sh", "-c", cmd}
		cmd := exec.Cmd{
			Path: args[0],
			Args: args,
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			panic(err)
		}
		success := false
		if cmd.ProcessState != nil {
			success = cmd.ProcessState.Success()
		}

		fmt.Printf("%s", out)
		if !success {
			panic("Command failed")
		}
	}
}

func (ex *Executor) build(output string) int64 {
	Log("Building: %s", output)
	output_ts := getTimestamp(output)

	rule, present := ex.rules[output]
	if !present {
		if output_ts >= 0 {
			return output_ts
		}
		Error("No rule to make target '%s'", output)
	}

	latest := int64(-1)
	for _, input := range rule.inputs {
		ts := ex.build(input)
		if latest < ts {
			latest = ts
		}
	}

	if output_ts >= latest {
		return output_ts
	}

	ex.runCommands(rule.cmds)

	output_ts = getTimestamp(output)
	if output_ts < 0 {
		output_ts = time.Now().Unix()
	}
	return output_ts
}

func (ex *Executor) exec(er *EvalResult) {
	if len(er.rules) == 0 {
		panic("No targets.")
	}

	for _, rule := range er.rules {
		if _, present := ex.rules[rule.output]; present {
			Warn("overiding recipie for target '%s'", rule.output)
		}
		ex.rules[rule.output] = rule
	}

	ex.build(er.rules[0].output)
}

func Exec(er *EvalResult) {
	ex := newExecutor()
	ex.exec(er)
}
