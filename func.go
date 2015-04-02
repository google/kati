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
// TODO(ukai): return error instead of panic?
// TODO(ukai): each func has nargs, and don't split , more than narg?
type Func func(*Evaluator, []string) string

func funcSubst(ev *Evaluator, args []string) string {
	Log("subst %q", args)
	if len(args) < 3 {
		panic(fmt.Sprintf("*** insufficient number of arguments (%d) to function `subst'.", len(args)))
	}
	from := ev.evalExpr(args[0])
	to := ev.evalExpr(args[1])
	text := ev.evalExpr(strings.Join(args[2:], ","))
	Log("subst from:%q to:%q text:%q", from, to, text)
	return strings.Replace(text, from, to, -1)
}

func funcPatsubst(ev *Evaluator, args []string) string {
	Log("patsubst %q", args)
	if len(args) < 3 {
		panic(fmt.Sprintf("*** insufficient number of arguments (%d) to function `patsubst'.", len(args)))
	}
	pat := ev.evalExpr(args[0])
	repl := ev.evalExpr(args[1])
	texts := splitSpaces(ev.evalExpr(strings.Join(args[2:], ",")))
	for i, text := range texts {
		texts[i] = substPattern(pat, repl, text)
	}
	return strings.Join(texts, " ")
}

// http://www.gnu.org/software/make/manual/make.html#File-Name-Functions
func funcWildcard(ev *Evaluator, args []string) string {
	Log("wildcard %q", args)
	pattern := ev.evalExpr(strings.Join(args, ","))
	files, err := filepath.Glob(pattern)
	if err != nil {
		panic(err)
	}
	return strings.Join(files, " ")
}

// https://www.gnu.org/software/make/manual/html_node/File-Name-Functions.html#File-Name-Functions
func funcDir(ev *Evaluator, args []string) string {
	Log("dir %q", args)
	name := ev.evalExpr(strings.Join(args, ","))
	if name == "" {
		return ""
	}
	return filepath.Dir(name) + string(filepath.Separator)
}

func funcNotdir(ev *Evaluator, args []string) string {
	Log("notdir %q", args)
	name := ev.evalExpr(strings.Join(args, ","))
	if name == "" || name == string(filepath.Separator) {
		return ""
	}
	return filepath.Base(name)
}

func funcRealpath(ev *Evaluator, args []string) string {
	Log("realpath %q", args)
	names := strings.Split(ev.evalExpr(strings.Join(args, ",")), " \t")
	var realpaths []string
	for _, name := range names {
		name = strings.TrimSpace(name)
		name, err := filepath.Abs(name)
		if err != nil {
			Log("abs: %v", err)
			continue
		}
		name, err = filepath.EvalSymlinks(name)
		if err != nil {
			Log("realpath: %v", err)
			continue
		}
		realpaths = append(realpaths, name)
	}
	return strings.Join(realpaths, " ")
}

func funcAbspath(ev *Evaluator, args []string) string {
	Log("abspath %q", args)
	names := strings.Split(ev.evalExpr(strings.Join(args, ",")), " \t")
	var realpaths []string
	for _, name := range names {
		name = strings.TrimSpace(name)
		name, err := filepath.Abs(name)
		if err != nil {
			Log("abs: %v", err)
			continue
		}
		realpaths = append(realpaths, name)
	}
	return strings.Join(realpaths, " ")
}

// http://www.gnu.org/software/make/manual/make.html#Shell-Function
func funcShell(ev *Evaluator, args []string) string {
	Log("shell %q", args)
	arg := ev.evalExpr(strings.Join(args, ","))
	cmdline := []string{"/bin/sh", "-c", arg}
	cmd := exec.Cmd{
		Path: cmdline[0],
		Args: cmdline,
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

// https://www.gnu.org/software/make/manual/html_node/Call-Function.html#Call-Function
func funcCall(ev *Evaluator, args []string) string {
	Log("call %q", args)
	f := ev.LookupVar(args[0]).String()
	Log("call func %q => %q", args[0], f)
	localVars := NewVarTab(ev.VarTab())
	for i, argstr := range args[1:] {
		arg := ev.evalExpr(argstr)
		Log("call $%d: %q=>%q", i+1, argstr, arg)
		localVars.Assign(fmt.Sprintf("%d", i+1),
			RecursiveVar{
				expr:   arg,
				origin: "automatic", // ??
			})
	}
	ev = newEvaluator(localVars)
	r := ev.evalExpr(f)
	Log("call %q return %q", args[0], r)
	return r
}

// https://www.gnu.org/software/make/manual/html_node/Flavor-Function.html#Flavor-Function
func funcFlavor(ev *Evaluator, args []string) string {
	Log("flavor %q", args)
	vname := strings.Join(args, ",")
	return ev.LookupVar(vname).Flavor()
}

// http://www.gnu.org/software/make/manual/make.html#Make-Control-Functions
func funcWarning(ev *Evaluator, args []string) string {
	Log("warning %q", args)
	arg := ev.evalExpr(strings.Join(args, ","))
	fmt.Printf("%s:%d: %s\n", ev.filename, ev.lineno, arg)
	return ""
}
