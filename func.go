package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// TODO(ukai): move in var.go?
type oldVar struct {
	name  string
	value Var
}

func newOldVar(ev *Evaluator, name string) oldVar {
	return oldVar{
		name:  name,
		value: ev.outVars.Lookup(name),
	}
}

func (old oldVar) restore(ev *Evaluator) {
	if old.value.IsDefined() {
		ev.outVars.Assign(old.name, old.value)
		return
	}
	delete(ev.outVars, old.name)
}

// Func is a make function.
// http://www.gnu.org/software/make/manual/make.html#Functions

// Func is make builtin function.
type Func interface {
	// Arity is max function's arity.
	// ',' will not be handled as argument separator more than arity.
	// 0 means varargs.
	Arity() int

	// AddArg adds value as an argument.
	AddArg(Value)

	Value
}

var (
	funcMap = map[string]func() Func{
		"subst": func() Func { return &funcSubst{} },
		"shell": func() Func { return &funcShell{} },
	}
)

func init() {
	fwrap("patsubst", 3, funcPatsubst)
	fwrap("strip", 1, funcStrip)
	fwrap("findstring", 2, funcFindstring)
	fwrap("filter", 2, funcFilter)
	fwrap("filter-out", 2, funcFilterOut)
	fwrap("sort", 1, funcSort)
	fwrap("word", 2, funcWord)
	fwrap("wordlist", 3, funcWordlist)
	fwrap("words", 1, funcWords)
	fwrap("firstword", 1, funcFirstword)
	fwrap("lastword", 1, funcLastword)
	fwrap("join", 2, funcJoin)
	fwrap("wildcard", 1, funcWildcard)
	fwrap("dir", 1, funcDir)
	fwrap("notdir", 1, funcNotdir)
	fwrap("suffix", 1, funcSuffix)
	fwrap("basename", 1, funcBasename)
	fwrap("addsuffix", 2, funcAddsuffix)
	fwrap("addprefix", 2, funcAddprefix)
	fwrap("realpath", 1, funcRealpath)
	fwrap("abspath", 1, funcAbspath)
	fwrap("if", 3, funcIf)
	fwrap("and", 0, funcAnd)
	fwrap("or", 0, funcOr)
	fwrap("foreach", 3, funcForeach)
	fwrap("value", 1, funcValue)
	fwrap("eval", 1, funcEval)
	fwrap("origin", 1, funcOrigin)
	fwrap("call", 0, funcCall)
	fwrap("flavor", 1, funcFlavor)
	fwrap("info", 1, funcInfo)
	fwrap("warning", 1, funcWarning)
	fwrap("error", 1, funcError)
}

func assertArity(name string, req, n int) {
	if n < req {
		panic(fmt.Sprintf("*** insufficient number of arguments (%d) to function `%s'.", n, name))
	}
}

type fclosure struct {
	args []Value
}

func (c *fclosure) AddArg(v Value) {
	c.args = append(c.args, v)
}

// http://www.gnu.org/software/make/manual/make.html#Text-Functions
type funcSubst struct{ fclosure }

func (f *funcSubst) Arity() int { return 3 }
func (f *funcSubst) String() string {
	return fmt.Sprintf("${subst %s,%s,%s}", f.args[0], f.args[1], f.args[2])
}

func (f *funcSubst) Eval(w io.Writer, ev *Evaluator) {
	assertArity("subst", 3, len(f.args))
	from := ev.Value(f.args[0])
	to := ev.Value(f.args[1])
	text := ev.Value(f.args[2])
	Log("subst from:%q to:%q text:%q", from, to, text)
	w.Write(bytes.Replace(text, from, to, -1))
}

// http://www.gnu.org/software/make/manual/make.html#Shell-Function
type funcShell struct{ fclosure }

func (f *funcShell) Arity() int { return 1 }
func (f *funcShell) String() string {
	// TODO(ukai): Crash by ./run_integration_tests.rb android
	//return fmt.Sprintf("${shell %s}", f.args[1])
	return ""
}

func (f *funcShell) Eval(w io.Writer, ev *Evaluator) {
	assertArity("shell", 1, len(f.args))
	arg := ev.Value(f.args[0])
	cmdline := []string{"/bin/sh", "-c", string(arg)}
	cmd := exec.Cmd{
		Path:   cmdline[0],
		Args:   cmdline,
		Stderr: os.Stderr,
	}
	out, err := cmd.Output()
	if err != nil {
		Log("$(shell %q) failed: %q", arg, err)
	}

	r := string(out)
	r = strings.TrimRight(r, "\n")
	r = strings.Replace(r, "\n", " ", -1)
	fmt.Fprint(w, r)
}

// TODO(ukai): rewrite new style func.

type fwrapclosure struct {
	fclosure
	name  string
	arity int
	f     func(ev *Evaluator, args []string) string
}

func (f *fwrapclosure) Arity() int {
	return f.arity
}

func (f *fwrapclosure) String() string {
	var args []string
	for _, arg := range f.args {
		args = append(args, arg.String())
	}
	return fmt.Sprintf("${%s %s}", f.name, strings.Join(args, ","))
}

func (f *fwrapclosure) Eval(w io.Writer, ev *Evaluator) {
	var args []string
	for _, arg := range f.args {
		args = append(args, arg.String())
	}
	r := f.f(ev, args)
	fmt.Fprint(w, r)
}

func fwrap(name string, arity int, f func(ev *Evaluator, args []string) string) {
	funcMap[name] = func() Func {
		return &fwrapclosure{
			name:  name,
			arity: arity,
			f:     f,
		}
	}
}

func arity(name string, req int, args []string) []string {
	assertArity(name, req, len(args))
	args[req-1] = strings.Join(args[req-1:], ",")
	return args
}

func funcPatsubst(ev *Evaluator, args []string) string {
	args = arity("patsubst", 3, args)
	pat := ev.evalExpr(args[0])
	repl := ev.evalExpr(args[1])
	texts := splitSpaces(ev.evalExpr(args[2]))
	for i, text := range texts {
		texts[i] = substPattern(pat, repl, text)
	}
	return strings.Join(texts, " ")
}

func funcStrip(ev *Evaluator, args []string) string {
	args = arity("strip", 1, args)
	text := ev.evalExpr(args[0])
	return strings.TrimSpace(text)
}

func funcFindstring(ev *Evaluator, args []string) string {
	args = arity("findstring", 2, args)
	f := ev.evalExpr(args[0])
	text := ev.evalExpr(args[1])
	if strings.Index(text, f) >= 0 {
		return f
	}
	return ""
}

func funcFilter(ev *Evaluator, args []string) string {
	args = arity("filter", 2, args)
	patterns := splitSpaces(ev.evalExpr(args[0]))
	texts := splitSpaces(ev.evalExpr(args[1]))
	var result []string
	for _, text := range texts {
		for _, pat := range patterns {
			if matchPattern(pat, text) {
				result = append(result, text)
			}
		}
	}
	return strings.Join(result, " ")
}

func funcFilterOut(ev *Evaluator, args []string) string {
	args = arity("filter-out", 2, args)
	patterns := splitSpaces(ev.evalExpr(args[0]))
	texts := splitSpaces(ev.evalExpr(args[1]))
	var result []string
Loop:
	for _, text := range texts {
		for _, pat := range patterns {
			if matchPattern(pat, text) {
				continue Loop
			}
		}
		result = append(result, text)
	}
	return strings.Join(result, " ")
}

func funcSort(ev *Evaluator, args []string) string {
	args = arity("sort", 1, args)
	toks := splitSpaces(ev.evalExpr(args[0]))
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
	args = arity("word", 2, args)
	index := numericValueForFunc(ev, args[0], "word", "first")
	if index == 0 {
		Error(ev.filename, ev.lineno, `*** first argument to "word" function must be greater than 0.`)
	}
	toks := splitSpaces(ev.evalExpr(args[1]))
	if index-1 >= len(toks) {
		return ""
	}
	return ev.evalExpr(toks[index-1])
}

func funcWordlist(ev *Evaluator, args []string) string {
	args = arity("wordlist", 3, args)
	si := numericValueForFunc(ev, args[0], "wordlist", "first")
	if si == 0 {
		Error(ev.filename, ev.lineno, `*** invalid first argument to "wordlist" function: ""`, args[0])
	}
	ei := numericValueForFunc(ev, args[1], "wordlist", "second")
	if ei == 0 {
		Error(ev.filename, ev.lineno, `*** invalid second argument to "wordlist" function: ""`, args[1])
	}

	toks := splitSpaces(ev.evalExpr(args[2]))
	if si-1 >= len(toks) {
		return ""
	}
	if ei-1 >= len(toks) {
		ei = len(toks)
	}

	return strings.Join(toks[si-1:ei], " ")
}

func funcWords(ev *Evaluator, args []string) string {
	args = arity("words", 1, args)
	toks := splitSpaces(ev.evalExpr(args[0]))
	return strconv.Itoa(len(toks))
}

func funcFirstword(ev *Evaluator, args []string) string {
	args = arity("firstword", 1, args)
	toks := splitSpaces(ev.evalExpr(args[0]))
	if len(toks) == 0 {
		return ""
	}
	return toks[0]
}

func funcLastword(ev *Evaluator, args []string) string {
	args = arity("lastword", 1, args)
	toks := splitSpaces(ev.evalExpr(args[0]))
	if len(toks) == 0 {
		return ""
	}
	return toks[len(toks)-1]
}

// http://www.gnu.org/software/make/manual/make.html#File-Name-Functions
func funcJoin(ev *Evaluator, args []string) string {
	args = arity("join", 2, args)
	list1 := splitSpaces(ev.evalExpr(args[0]))
	list2 := splitSpaces(ev.evalExpr(args[1]))
	var results []string
	for i, v := range list1 {
		if i < len(list2) {
			results = append(results, v+list2[i])
			continue
		}
		results = append(results, v)
	}
	if len(list2) > len(list1) {
		for _, v := range list2[len(list1):] {
			results = append(results, v)
		}
	}
	return strings.Join(results, " ")
}

func funcWildcard(ev *Evaluator, args []string) string {
	args = arity("wildcard", 1, args)
	var result []string
	for _, pattern := range splitSpaces(ev.evalExpr(args[0])) {
		files, err := filepath.Glob(pattern)
		if err != nil {
			panic(err)
		}
		result = append(result, files...)
	}
	return strings.Join(result, " ")
}

// https://www.gnu.org/software/make/manual/html_node/File-Name-Functions.html#File-Name-Functions
func funcDir(ev *Evaluator, args []string) string {
	args = arity("dir", 1, args)
	names := splitSpaces(ev.evalExpr(args[0]))
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
	args = arity("notdir", 1, args)
	names := splitSpaces(ev.evalExpr(args[0]))
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
	args = arity("suffix", 1, args)
	toks := splitSpaces(ev.evalExpr(args[0]))
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
	args = arity("basename", 1, args)
	toks := splitSpaces(ev.evalExpr(args[0]))
	var result []string
	for _, tok := range toks {
		b := stripExt(tok)
		result = append(result, b)
	}
	return strings.Join(result, " ")
}

func funcAddsuffix(ev *Evaluator, args []string) string {
	args = arity("addsuffix", 2, args)
	suf := ev.evalExpr(args[0])
	toks := splitSpaces(ev.evalExpr(args[1]))
	for i, tok := range toks {
		toks[i] = fmt.Sprintf("%s%s", tok, suf)
	}
	return strings.Join(toks, " ")
}

func funcAddprefix(ev *Evaluator, args []string) string {
	args = arity("addprefix", 2, args)
	pre := ev.evalExpr(args[0])
	toks := splitSpaces(ev.evalExpr(args[1]))
	for i, tok := range toks {
		toks[i] = fmt.Sprintf("%s%s", pre, tok)
	}
	return strings.Join(toks, " ")
}

func funcRealpath(ev *Evaluator, args []string) string {
	args = arity("realpath", 1, args)
	names := splitSpaces(ev.evalExpr(args[0]))
	var realpaths []string
	for _, name := range names {
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
	args = arity("abspath", 1, args)
	names := splitSpaces(ev.evalExpr(args[0]))
	var realpaths []string
	for _, name := range names {
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
	if len(args) < 2 {
		panic(fmt.Sprintf("*** insufficient number of arguments (%2) to function `if'.", len(args)))
	}
	cond := ev.evalExpr(strings.TrimSpace(args[0]))
	if cond != "" {
		return ev.evalExpr(args[1])
	}
	var results []string
	for _, part := range args[2:] {
		results = append(results, ev.evalExpr(part))
	}
	return strings.Join(results, ",")
}

func funcOr(ev *Evaluator, args []string) string {
	for _, arg := range args {
		cond := ev.evalExpr(strings.TrimSpace(arg))
		if cond != "" {
			return cond
		}
	}
	return ""
}

func funcAnd(ev *Evaluator, args []string) string {
	var cond string
	for _, arg := range args {
		cond = ev.evalExpr(strings.TrimSpace(arg))
		if cond == "" {
			return ""
		}
	}
	return cond
}

// http://www.gnu.org/software/make/manual/make.html#Foreach-Function
func funcForeach(ev *Evaluator, args []string) string {
	args = arity("foreach", 3, args)
	var result []string
	varName := ev.evalExpr(args[0])
	values := splitSpaces(ev.evalExpr(args[1]))
	expr := args[2]
	old := newOldVar(ev, varName)
	for _, val := range values {
		ev.outVars.Assign(varName,
			SimpleVar{
				value:  val,
				origin: "automatic",
			})
		result = append(result, ev.evalExpr(expr))
	}
	old.restore(ev)
	return strings.Join(result, " ")
}

// http://www.gnu.org/software/make/manual/make.html#Value-Function
func funcValue(ev *Evaluator, args []string) string {
	args = arity("value", 1, args)
	v := ev.LookupVar(args[0])
	return v.String()
}

// http://www.gnu.org/software/make/manual/make.html#Eval-Function
func funcEval(ev *Evaluator, args []string) string {
	args = arity("eval", 1, args)
	s := ev.evalExpr(args[0])
	if s == "" || (s[0] == '#' && strings.IndexByte(s, '\n') < 0) {
		return ""
	}
	mk, err := ParseMakefileString(s, ev.filename, ev.lineno)
	if err != nil {
		panic(err)
	}

	for _, stmt := range mk.stmts {
		ev.eval(stmt)
	}

	return ""
}

// http://www.gnu.org/software/make/manual/make.html#Origin-Function
func funcOrigin(ev *Evaluator, args []string) string {
	args = arity("origin", 1, args)
	v := ev.LookupVar(args[0])
	return v.Origin()
}

// https://www.gnu.org/software/make/manual/html_node/Call-Function.html#Call-Function
func funcCall(ev *Evaluator, args []string) string {
	f := ev.LookupVar(args[0]).String()
	Log("call func %q => %q", args[0], f)
	// Evalualte all arguments first before we modify the table.
	for i, argstr := range args[1:] {
		args[i+1] = ev.evalExpr(argstr)
		Log("call $%d: %q=>%q", i+1, argstr, args[i])
	}

	var olds []oldVar
	for i, arg := range args[1:] {
		name := fmt.Sprintf("%d", i+1)
		olds = append(olds, newOldVar(ev, name))
		ev.outVars.Assign(name,
			RecursiveVar{
				expr:   tmpval([]byte(arg)),
				origin: "automatic", // ??
			})
	}

	r := ev.evalExpr(f)
	for _, old := range olds {
		old.restore(ev)
	}
	Log("call %q return %q", args[0], r)
	return r
}

// https://www.gnu.org/software/make/manual/html_node/Flavor-Function.html#Flavor-Function
func funcFlavor(ev *Evaluator, args []string) string {
	args = arity("flavor", 1, args)
	vname := args[0]
	return ev.LookupVar(vname).Flavor()
}

// http://www.gnu.org/software/make/manual/make.html#Make-Control-Functions
func funcInfo(ev *Evaluator, args []string) string {
	args = arity("info", 1, args)
	arg := ev.evalExpr(args[0])
	fmt.Printf("%s\n", arg)
	return ""
}

func funcWarning(ev *Evaluator, args []string) string {
	args = arity("warning", 1, args)
	arg := ev.evalExpr(args[0])
	fmt.Printf("%s:%d: %s\n", ev.filename, ev.lineno, arg)
	return ""
}

func funcError(ev *Evaluator, args []string) string {
	args = arity("error", 1, args)
	arg := ev.evalExpr(args[0])
	Error(ev.filename, ev.lineno, "*** %s.", arg)
	return ""
}
