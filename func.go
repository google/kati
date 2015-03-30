package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Func is a make function.
// http://www.gnu.org/software/make/manual/make.html#Functions
// TODO(ukai): *Evaluator -> eval context or so?
// TODO(ukai): return error instead of panic?
type Func func(*Evaluator, string) string

// http://www.gnu.org/software/make/manual/make.html#File-Name-Functions
func funcWildcard(_ *Evaluator, arg string) string {
	files, err := filepath.Glob(arg)
	if err != nil {
		panic(err)
	}
	return strings.Join(files, " ")
}

// http://www.gnu.org/software/make/manual/make.html#Shell-Function
func funcShell(_ *Evaluator, arg string) string {
	args := []string{"/bin/sh", "-c", arg}
	cmd := exec.Cmd{
		Path: args[0],
		Args: args,
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		panic(err)
	}
	re, err := regexp.Compile(`\s`)
	if err != nil {
		panic(err)
	}
	return string(re.ReplaceAllString(string(out), " "))
}

// http://www.gnu.org/software/make/manual/make.html#Make-Control-Functions
func funcWarning(ev *Evaluator, arg string) string {
	fmt.Printf("%s:%d: %s\n", ev.filename, ev.lineno, arg)
	return ""
}
