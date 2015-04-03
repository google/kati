package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Func is a make function.
// http://www.gnu.org/software/make/manual/make.html#Functions
// TODO(ukai): return error instead of panic?
// TODO(ukai): each func has nargs, and don't split , more than narg?
type Func func(*Evaluator, []string) string

// http://www.gnu.org/software/make/manual/make.html#Text-Functions
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

func funcStrip(ev *Evaluator, args []string) string {
	text := ev.evalExpr(strings.Join(args, ","))
	return strings.TrimSpace(text)
}

func funcFindstring(ev *Evaluator, args []string) string {
	if len(args) < 2 {
		panic(fmt.Sprintf("*** insufficient number of arguments (%d) to function `findstring'.", len(args)))
	}
	f := ev.evalExpr(args[0])
	text := ev.evalExpr(strings.Join(args[1:], ","))
	if strings.Index(text, f) >= 0 {
		return f
	}
	return ""
}

// filter
// filter-out

func funcSort(ev *Evaluator, args []string) string {
	if len(args) <= 0 {
		panic(fmt.Sprintf("*** insufficient number of arguments (%d) to function `sort'.", len(args)))
	}
	toks := splitSpaces(ev.evalExpr(strings.Join(args, ",")))
	sort.Strings(toks)

	// Remove duplicate words.
	var prev string
	var result []string
	for _, tok := range toks {
		if prev != tok {
			result = append(result, tok)
			prev = tok
		}
	}
	return strings.Join(result, " ")
}

func numericValueForFunc(ev *Evaluator, a string, funcName string, nth string) int {
	a = strings.TrimSpace(ev.evalExpr(a))
	n, err := strconv.Atoi(a)
	if err != nil || n < 0 {
		Error(ev.filename, ev.lineno, `*** non-numeric %s argument to "%s" function: "%s".`, nth, funcName, a)
	}
	return n
}

func funcWord(ev *Evaluator, args []string) string {
	if len(args) < 2 {
		panic(fmt.Sprintf("*** insufficient number of arguments (%d) to function `word'.", len(args)))
	}
	index := numericValueForFunc(ev, args[0], "word", "first")
	if index == 0 {
		Error(ev.filename, ev.lineno, `*** first argument to "word" function must be greater than 0.`)
	}
	toks := splitSpaces(ev.evalExpr(strings.Join(args[1:], ",")))
	if index-1 >= len(toks) {
		return ""
	}
	return ev.evalExpr(toks[index-1])
}

func funcWordlist(ev *Evaluator, args []string) string {
	if len(args) < 3 {
		panic(fmt.Sprintf("*** insufficient number of arguments (%d) to function `wordlist'.", len(args)))
	}
	si := numericValueForFunc(ev, args[0], "wordlist", "first")
	if si == 0 {
		Error(ev.filename, ev.lineno, `*** invalid first argument to "wordlist" function: ""`, args[0])
	}
	ei := numericValueForFunc(ev, args[1], "wordlist", "second")
	if ei == 0 {
		Error(ev.filename, ev.lineno, `*** invalid second argument to "wordlist" function: ""`, args[1])
	}

	toks := splitSpaces(ev.evalExpr(strings.Join(args[2:], ",")))
	if si-1 >= len(toks) {
		return ""
	}
	if ei-1 >= len(toks) {
		ei = len(toks)
	}

	return strings.Join(toks[si-1:ei], " ")
}

func funcWords(ev *Evaluator, args []string) string {
	toks := splitSpaces(ev.evalExpr(strings.Join(args, ",")))
	return strconv.Itoa(len(toks))
}

func funcFirstword(ev *Evaluator, args []string) string {
	toks := splitSpaces(ev.evalExpr(strings.Join(args, ",")))
	if len(toks) == 0 {
		return ""
	}
	return toks[0]
}

func funcLastword(ev *Evaluator, args []string) string {
	toks := splitSpaces(ev.evalExpr(strings.Join(args, ",")))
	if len(toks) == 0 {
		return ""
	}
	return toks[len(toks)-1]
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
	names := splitSpaces(ev.evalExpr(strings.Join(args, ",")))
	if len(names) == 0 {
		return ""
	}
	var dirs []string
	for _, name := range names {
		dirs = append(dirs, filepath.Dir(name)+string(filepath.Separator))
	}
	return strings.Join(dirs, " ")
}

func funcNotdir(ev *Evaluator, args []string) string {
	Log("notdir %q", args)
	names := splitSpaces(ev.evalExpr(strings.Join(args, ",")))
	if len(names) == 0 {
		return ""
	}
	var notdirs []string
	for _, name := range names {
		if name == string(filepath.Separator) {
			notdirs = append(notdirs, "")
			continue
		}
		notdirs = append(notdirs, filepath.Base(name))
	}
	return strings.Join(notdirs, " ")
}

func funcSuffix(ev *Evaluator, args []string) string {
	toks := splitSpaces(ev.evalExpr(strings.Join(args, ",")))
	var result []string
	for _, tok := range toks {
		e := filepath.Ext(tok)
		if len(e) > 0 {
			result = append(result, e)
		}
	}
	return strings.Join(result, " ")
}

func funcBasename(ev *Evaluator, args []string) string {
	toks := splitSpaces(ev.evalExpr(strings.Join(args, ",")))
	var result []string
	for _, tok := range toks {
		b := stripExt(tok)
		result = append(result, b)
	}
	return strings.Join(result, " ")
}

func funcAddsuffix(ev *Evaluator, args []string) string {
	if len(args) < 2 {
		panic(fmt.Sprintf("*** insufficient number of arguments (%d) to function `addsuffix'.", len(args)))
	}
	suf := ev.evalExpr(args[0])
	toks := splitSpaces(ev.evalExpr(strings.Join(args[1:], ",")))
	for i, tok := range toks {
		toks[i] = fmt.Sprintf("%s%s", tok, suf)
	}
	return strings.Join(toks, " ")
}

func funcAddprefix(ev *Evaluator, args []string) string {
	if len(args) < 2 {
		panic(fmt.Sprintf("*** insufficient number of arguments (%d) to function `addprefix'.", len(args)))
	}
	pre := ev.evalExpr(args[0])
	toks := splitSpaces(ev.evalExpr(strings.Join(args[1:], ",")))
	for i, tok := range toks {
		toks[i] = fmt.Sprintf("%s%s", pre, tok)
	}
	return strings.Join(toks, " ")
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

// http://www.gnu.org/software/make/manual/make.html#Conditional-Functions
func funcIf(ev *Evaluator, args []string) string {
	if len(args) < 3 {
		panic(fmt.Sprintf("*** insufficient number of arguments (%d) to function `if'.", len(args)))
	}
	cond := ev.evalExpr(args[0])
	if len(cond) > 0 {
		return ev.evalExpr(args[1])
	} else {
		return ev.evalExpr(strings.Join(args[2:], ","))
	}
}

func funcOr(ev *Evaluator, args []string) string {
	if len(args) < 2 {
		panic(fmt.Sprintf("*** insufficient number of arguments (%d) to function `or'.", len(args)))
	}
	cond := ev.evalExpr(args[0])
	if len(cond) == 0 {
		// For some reason, "and" and "or" do not use args[2:]
		return ev.evalExpr(strings.TrimSpace(args[1]))
	}
	return cond
}

func funcAnd(ev *Evaluator, args []string) string {
	if len(args) < 2 {
		panic(fmt.Sprintf("*** insufficient number of arguments (%d) to function `and'.", len(args)))
	}
	cond := ev.evalExpr(args[0])
	if len(cond) > 0 {
		// For some reason, "and" and "or" do not use args[2:]
		return ev.evalExpr(strings.TrimSpace(args[1]))
	}
	return ""
}

// http://www.gnu.org/software/make/manual/make.html#Foreach-Function
func funcForeach(ev *Evaluator, args []string) string {
	if len(args) < 3 {
		panic(fmt.Sprintf("*** insufficient number of arguments (%d) to function `foreach'.", len(args)))
	}
	var result []string
	varName := ev.evalExpr(args[0])
	values := splitSpaces(ev.evalExpr(args[1]))
	expr := strings.Join(args[2:], ",")
	for _, val := range values {
		newVars := NewVarTab(ev.vars)
		newVars.Assign(varName,
			SimpleVar{
				value:  val,
				origin: "automatic",
			})
		oldVars := ev.vars
		ev.vars = newVars
		result = append(result, ev.evalExpr(expr))
		ev.vars = oldVars
	}
	return strings.Join(result, " ")
}

// http://www.gnu.org/software/make/manual/make.html#Eval-Function
func funcEval(ev *Evaluator, args []string) string {
	s := ev.evalExpr(strings.Join(args, ","))
	mk, err := ParseMakefileString(s, "*eval*")
	if err != nil {
		panic(err)
	}

	er, err2 := Eval(mk, ev.VarTab())
	if err2 != nil {
		panic(err2)
	}

	for k, v := range er.vars.Vars() {
		ev.outVars.Assign(k, v)
	}
	for _, r := range er.rules {
		ev.outRules = append(ev.outRules, r)
	}

	return ""
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
		Log("$(shell %q) failed: %q", args, err)
	}

	r := string(out)
	r = strings.TrimRight(r, "\n")
	return strings.Replace(r, "\n", " ", -1)
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
func funcInfo(ev *Evaluator, args []string) string {
	Log("warning %q", args)
	arg := ev.evalExpr(strings.Join(args, ","))
	fmt.Printf("%s\n", arg)
	return ""
}

func funcWarning(ev *Evaluator, args []string) string {
	Log("warning %q", args)
	arg := ev.evalExpr(strings.Join(args, ","))
	fmt.Printf("%s:%d: %s\n", ev.filename, ev.lineno, arg)
	return ""
}

func funcError(ev *Evaluator, args []string) string {
	Log("warning %q", args)
	arg := ev.evalExpr(strings.Join(args, ","))
	Error(ev.filename, ev.lineno, "*** %s.", arg)
	return ""
}
