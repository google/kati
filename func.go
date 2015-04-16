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

// Func is a make function.
// http://www.gnu.org/software/make/manual/make.html#Functions

// Func is make builtin function.
type Func interface {
	// Arity is max function's arity.
	// ',' will not be handled as argument separator more than arity.
	// 0 means varargs.
	Arity() int

	// AddArg adds value as an argument.
	// the first argument will be "(funcname", or "{funcname".
	AddArg(Value)

	Value
}

var (
	funcMap = map[string]func() Func{
		"patsubst":   func() Func { return &funcPatsubst{} },
		"strip":      func() Func { return &funcStrip{} },
		"subst":      func() Func { return &funcSubst{} },
		"findstring": func() Func { return &funcFindstring{} },
		"filter":     func() Func { return &funcFilter{} },
		"filter-out": func() Func { return &funcFilterOut{} },
		"sort":       func() Func { return &funcSort{} },
		"word":       func() Func { return &funcWord{} },
		"wordlist":   func() Func { return &funcWordlist{} },
		"words":      func() Func { return &funcWords{} },
		"firstword":  func() Func { return &funcFirstword{} },
		"lastword":   func() Func { return &funcLastword{} },

		"join":      func() Func { return &funcJoin{} },
		"wildcard":  func() Func { return &funcWildcard{} },
		"dir":       func() Func { return &funcDir{} },
		"notdir":    func() Func { return &funcNotdir{} },
		"suffix":    func() Func { return &funcSuffix{} },
		"basename":  func() Func { return &funcBasename{} },
		"addsuffix": func() Func { return &funcAddsuffix{} },
		"addprefix": func() Func { return &funcAddprefix{} },
		"realpath":  func() Func { return &funcRealpath{} },
		"abspath":   func() Func { return &funcAbspath{} },

		"if":  func() Func { return &funcIf{} },
		"and": func() Func { return &funcAnd{} },
		"or":  func() Func { return &funcOr{} },

		"value": func() Func { return &funcValue{} },

		"eval": func() Func { return &funcEval{} },

		"shell":   func() Func { return &funcShell{} },
		"call":    func() Func { return &funcCall{} },
		"foreach": func() Func { return &funcForeach{} },

		"origin":  func() Func { return &funcOrigin{} },
		"flavor":  func() Func { return &funcFlavor{} },
		"info":    func() Func { return &funcInfo{} },
		"warning": func() Func { return &funcWarning{} },
		"error":   func() Func { return &funcError{} },
	}
)

func assertArity(name string, req, n int) {
	if n-1 < req {
		panic(fmt.Sprintf("*** insufficient number of arguments (%d) to function `%s'.", n-1, name))
	}
}

// A space separated values writer.
type ssvWriter struct {
	w          io.Writer
	needsSpace bool
}

func (sw *ssvWriter) Write(b []byte) {
	if sw.needsSpace {
		sw.w.Write([]byte{' '})
	}
	sw.needsSpace = true
	sw.w.Write(b)
}

func (sw *ssvWriter) WriteString(s string) {
	// TODO: Ineffcient. Nice if we can remove the cast.
	sw.Write([]byte(s))
}

func numericValueForFunc(ev *Evaluator, v Value, funcName string, nth string) int {
	a := bytes.TrimSpace(ev.Value(v))
	n, err := strconv.Atoi(string(a))
	if err != nil || n < 0 {
		Error(ev.filename, ev.lineno, `*** non-numeric %s argument to "%s" function: "%s".`, nth, funcName, a)
	}
	return n
}

type fclosure struct {
	// args[0] is "(funcname", or "{funcname".
	args []Value
}

func (c *fclosure) AddArg(v Value) {
	c.args = append(c.args, v)
}

func (c *fclosure) String() string {
	if len(c.args) == 0 {
		panic("no args in func")
	}
	arg0 := c.args[0].String()
	if arg0 == "" {
		panic(fmt.Errorf("wrong format of arg0: %q", arg0))
	}
	cp := closeParen(arg0[0])
	if cp == 0 {
		panic(fmt.Errorf("wrong format of arg0: %q", arg0))
	}
	var args []string
	for _, arg := range c.args[1:] {
		args = append(args, arg.String())
	}
	return fmt.Sprintf("$%s %s%c", arg0, strings.Join(args, ","), cp)
}

// http://www.gnu.org/software/make/manual/make.html#Text-Functions
type funcSubst struct{ fclosure }

func (f *funcSubst) Arity() int { return 3 }
func (f *funcSubst) Eval(w io.Writer, ev *Evaluator) {
	assertArity("subst", 3, len(f.args))
	from := ev.Value(f.args[1])
	to := ev.Value(f.args[2])
	text := ev.Value(f.args[3])
	Log("subst from:%q to:%q text:%q", from, to, text)
	w.Write(bytes.Replace(text, from, to, -1))
}

type funcPatsubst struct{ fclosure }

func (f *funcPatsubst) Arity() int { return 3 }
func (f *funcPatsubst) Eval(w io.Writer, ev *Evaluator) {
	assertArity("patsubst", 3, len(f.args))
	pat := ev.Value(f.args[1])
	repl := ev.Value(f.args[2])
	texts := splitSpacesBytes(ev.Value(f.args[3]))
	sw := ssvWriter{w: w}
	for _, text := range texts {
		t := substPatternBytes(pat, repl, text)
		sw.Write(t)
	}
}

type funcStrip struct{ fclosure }

func (f *funcStrip) Arity() int { return 1 }
func (f *funcStrip) Eval(w io.Writer, ev *Evaluator) {
	assertArity("strip", 1, len(f.args))
	text := ev.Value(f.args[1])
	w.Write(bytes.TrimSpace(text))
}

type funcFindstring struct{ fclosure }

func (f *funcFindstring) Arity() int { return 2 }
func (f *funcFindstring) Eval(w io.Writer, ev *Evaluator) {
	assertArity("findstring", 2, len(f.args))
	find := ev.Value(f.args[1])
	text := ev.Value(f.args[2])
	if bytes.Index(text, find) >= 0 {
		w.Write(find)
	}
}

type funcFilter struct{ fclosure }

func (f *funcFilter) Arity() int { return 2 }
func (f *funcFilter) Eval(w io.Writer, ev *Evaluator) {
	assertArity("filter", 2, len(f.args))
	patterns := splitSpacesBytes(ev.Value(f.args[1]))
	texts := splitSpacesBytes(ev.Value(f.args[2]))
	sw := ssvWriter{w: w}
	for _, text := range texts {
		for _, pat := range patterns {
			if matchPatternBytes(pat, text) {
				sw.Write(text)
			}
		}
	}
}

type funcFilterOut struct{ fclosure }

func (f *funcFilterOut) Arity() int { return 2 }
func (f *funcFilterOut) Eval(w io.Writer, ev *Evaluator) {
	assertArity("filter-out", 2, len(f.args))
	patterns := splitSpacesBytes(ev.Value(f.args[1]))
	texts := splitSpacesBytes(ev.Value(f.args[2]))
	sw := ssvWriter{w: w}
Loop:
	for _, text := range texts {
		for _, pat := range patterns {
			if matchPatternBytes(pat, text) {
				continue Loop
			}
		}
		sw.Write(text)
	}
}

type funcSort struct{ fclosure }

func (f *funcSort) Arity() int { return 1 }
func (f *funcSort) Eval(w io.Writer, ev *Evaluator) {
	assertArity("sort", 1, len(f.args))
	toks := splitSpaces(string(ev.Value(f.args[1])))
	sort.Strings(toks)

	// Remove duplicate words.
	var prev string
	sw := ssvWriter{w: w}
	for _, tok := range toks {
		if prev != tok {
			sw.WriteString(tok)
			prev = tok
		}
	}
}

type funcWord struct{ fclosure }

func (f *funcWord) Arity() int { return 2 }
func (f *funcWord) Eval(w io.Writer, ev *Evaluator) {
	assertArity("word", 2, len(f.args))
	index := numericValueForFunc(ev, f.args[1], "word", "first")
	if index == 0 {
		Error(ev.filename, ev.lineno, `*** first argument to "word" function must be greater than 0.`)
	}
	toks := splitSpacesBytes(ev.Value(f.args[2]))
	if index-1 >= len(toks) {
		return
	}
	w.Write(toks[index-1])
}

type funcWordlist struct{ fclosure }

func (f *funcWordlist) Arity() int { return 3 }
func (f *funcWordlist) Eval(w io.Writer, ev *Evaluator) {
	assertArity("wordlist", 3, len(f.args))
	si := numericValueForFunc(ev, f.args[1], "wordlist", "first")
	if si == 0 {
		Error(ev.filename, ev.lineno, `*** invalid first argument to "wordlist" function: %s`, f.args[1])
	}
	ei := numericValueForFunc(ev, f.args[2], "wordlist", "second")
	if ei == 0 {
		Error(ev.filename, ev.lineno, `*** invalid second argument to "wordlist" function: %s`, f.args[2])
	}

	toks := splitSpacesBytes(ev.Value(f.args[3]))
	if si-1 >= len(toks) {
		return
	}
	if ei-1 >= len(toks) {
		ei = len(toks)
	}

	sw := ssvWriter{w: w}
	for _, t := range toks[si-1 : ei] {
		sw.Write(t)
	}
}

type funcWords struct{ fclosure }

func (f *funcWords) Arity() int { return 1 }
func (f *funcWords) Eval(w io.Writer, ev *Evaluator) {
	assertArity("words", 1, len(f.args))
	toks := splitSpacesBytes(ev.Value(f.args[1]))
	w.Write([]byte(strconv.Itoa(len(toks))))
}

type funcFirstword struct{ fclosure }

func (f *funcFirstword) Arity() int { return 1 }
func (f *funcFirstword) Eval(w io.Writer, ev *Evaluator) {
	assertArity("firstword", 1, len(f.args))
	toks := splitSpacesBytes(ev.Value(f.args[1]))
	if len(toks) == 0 {
		return
	}
	w.Write(toks[0])
}

type funcLastword struct{ fclosure }

func (f *funcLastword) Arity() int { return 1 }
func (f *funcLastword) Eval(w io.Writer, ev *Evaluator) {
	assertArity("lastword", 1, len(f.args))
	toks := splitSpacesBytes(ev.Value(f.args[1]))
	if len(toks) == 0 {
		return
	}
	w.Write(toks[len(toks)-1])
}

// https://www.gnu.org/software/make/manual/html_node/File-Name-Functions.html#File-Name-Functions

type funcJoin struct{ fclosure }

func (f *funcJoin) Arity() int { return 2 }
func (f *funcJoin) Eval(w io.Writer, ev *Evaluator) {
	assertArity("join", 2, len(f.args))
	list1 := splitSpacesBytes(ev.Value(f.args[1]))
	list2 := splitSpacesBytes(ev.Value(f.args[2]))
	sw := ssvWriter{w: w}
	for i, v := range list1 {
		if i < len(list2) {
			sw.Write(v)
			// Use |w| not to append extra ' '.
			w.Write(list2[i])
			continue
		}
		sw.Write(v)
	}
	if len(list2) > len(list1) {
		for _, v := range list2[len(list1):] {
			sw.Write(v)
		}
	}
}

type funcWildcard struct{ fclosure }

func (f *funcWildcard) Arity() int { return 1 }
func (f *funcWildcard) Eval(w io.Writer, ev *Evaluator) {
	assertArity("wildcard", 1, len(f.args))
	sw := ssvWriter{w: w}
	for _, pattern := range splitSpaces(string(ev.Value(f.args[1]))) {
		files, err := filepath.Glob(pattern)
		if err != nil {
			panic(err)
		}
		for _, file := range files {
			sw.WriteString(file)
		}
	}
}

type funcDir struct{ fclosure }

func (f *funcDir) Arity() int { return 1 }
func (f *funcDir) Eval(w io.Writer, ev *Evaluator) {
	assertArity("dir", 1, len(f.args))
	names := splitSpaces(string(ev.Value(f.args[1])))
	sw := ssvWriter{w: w}
	for _, name := range names {
		sw.WriteString(filepath.Dir(name) + string(filepath.Separator))
	}
}

type funcNotdir struct{ fclosure }

func (f *funcNotdir) Arity() int { return 1 }
func (f *funcNotdir) Eval(w io.Writer, ev *Evaluator) {
	assertArity("notdir", 1, len(f.args))
	names := splitSpaces(string(ev.Value(f.args[1])))
	sw := ssvWriter{w: w}
	for _, name := range names {
		if name == string(filepath.Separator) {
			sw.Write([]byte{})
			continue
		}
		sw.WriteString(filepath.Base(name))
	}
}

type funcSuffix struct{ fclosure }

func (f *funcSuffix) Arity() int { return 1 }
func (f *funcSuffix) Eval(w io.Writer, ev *Evaluator) {
	assertArity("suffix", 1, len(f.args))
	toks := splitSpaces(string(ev.Value(f.args[1])))
	sw := ssvWriter{w: w}
	for _, tok := range toks {
		e := filepath.Ext(tok)
		if len(e) > 0 {
			sw.WriteString(e)
		}
	}
}

type funcBasename struct{ fclosure }

func (f *funcBasename) Arity() int { return 1 }
func (f *funcBasename) Eval(w io.Writer, ev *Evaluator) {
	assertArity("basename", 1, len(f.args))
	toks := splitSpaces(string(ev.Value(f.args[1])))
	sw := ssvWriter{w: w}
	for _, tok := range toks {
		e := stripExt(tok)
		sw.WriteString(e)
	}
}

type funcAddsuffix struct{ fclosure }

func (f *funcAddsuffix) Arity() int { return 2 }
func (f *funcAddsuffix) Eval(w io.Writer, ev *Evaluator) {
	assertArity("addsuffix", 2, len(f.args))
	suf := ev.Value(f.args[1])
	toks := splitSpacesBytes(ev.Value(f.args[2]))
	sw := ssvWriter{w: w}
	for _, tok := range toks {
		sw.Write(tok)
		// Use |w| not to append extra ' '.
		w.Write(suf)
	}
}

type funcAddprefix struct{ fclosure }

func (f *funcAddprefix) Arity() int { return 2 }
func (f *funcAddprefix) Eval(w io.Writer, ev *Evaluator) {
	assertArity("addprefix", 2, len(f.args))
	pre := ev.Value(f.args[1])
	toks := splitSpacesBytes(ev.Value(f.args[2]))
	sw := ssvWriter{w: w}
	for _, tok := range toks {
		sw.Write(pre)
		// Use |w| not to append extra ' '.
		w.Write(tok)
	}
}

type funcRealpath struct{ fclosure }

func (f *funcRealpath) Arity() int { return 1 }
func (f *funcRealpath) Eval(w io.Writer, ev *Evaluator) {
	assertArity("realpath", 1, len(f.args))
	names := splitSpaces(string(ev.Value(f.args[1])))
	sw := ssvWriter{w: w}
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
		sw.WriteString(name)
	}
}

type funcAbspath struct{ fclosure }

func (f *funcAbspath) Arity() int { return 1 }
func (f *funcAbspath) Eval(w io.Writer, ev *Evaluator) {
	assertArity("abspath", 1, len(f.args))
	names := splitSpaces(string(ev.Value(f.args[1])))
	sw := ssvWriter{w: w}
	for _, name := range names {
		name, err := filepath.Abs(name)
		if err != nil {
			Log("abs: %v", err)
			continue
		}
		sw.WriteString(name)
	}
}

// http://www.gnu.org/software/make/manual/make.html#Conditional-Functions
type funcIf struct{ fclosure }

func (f *funcIf) Arity() int { return 3 }
func (f *funcIf) Eval(w io.Writer, ev *Evaluator) {
	assertArity("if", 2, len(f.args))
	cond := ev.Value(f.args[1])
	if len(cond) != 0 {
		w.Write(ev.Value(f.args[2]))
		return
	}
	sw := ssvWriter{w: w}
	for _, part := range f.args[3:] {
		sw.Write(ev.Value(part))
	}
}

type funcAnd struct{ fclosure }

func (f *funcAnd) Arity() int { return 0 }
func (f *funcAnd) Eval(w io.Writer, ev *Evaluator) {
	assertArity("and", 0, len(f.args))
	var cond []byte
	for _, arg := range f.args[1:] {
		cond = ev.Value(arg)
		if len(cond) == 0 {
			return
		}
	}
	w.Write(cond)
}

type funcOr struct{ fclosure }

func (f *funcOr) Arity() int { return 0 }
func (f *funcOr) Eval(w io.Writer, ev *Evaluator) {
	assertArity("or", 0, len(f.args))
	for _, arg := range f.args[1:] {
		cond := ev.Value(arg)
		if len(cond) != 0 {
			w.Write(cond)
			return
		}
	}
}

// http://www.gnu.org/software/make/manual/make.html#Shell-Function
type funcShell struct{ fclosure }

func (f *funcShell) Arity() int { return 1 }

func (f *funcShell) Eval(w io.Writer, ev *Evaluator) {
	assertArity("shell", 1, len(f.args))
	arg := ev.Value(f.args[1])
	shellVar := ev.LookupVar("SHELL")
	// TODO: Should be Eval, not String.
	cmdline := []string{shellVar.String(), "-c", string(arg)}
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

// https://www.gnu.org/software/make/manual/html_node/Call-Function.html#Call-Function
type funcCall struct{ fclosure }

func (f *funcCall) Arity() int { return 0 }

func (f *funcCall) Eval(w io.Writer, ev *Evaluator) {
	variable := string(ev.Value(f.args[1]))
	v := ev.LookupVar(variable)
	Log("call variable %q", v)
	// Evalualte all arguments first before we modify the table.
	var args []tmpval
	for i, arg := range f.args[2:] {
		args = append(args, tmpval(ev.Value(arg)))
		Log("call $%d: %q=>%q", i+1, arg, args[i])
	}

	var restores []func()
	for i, arg := range args {
		name := fmt.Sprintf("%d", i+1)
		restores = append(restores, ev.outVars.save(name))
		ev.outVars.Assign(name,
			SimpleVar{
				value:  arg,
				origin: "automatic", // ??
			})
	}

	var buf bytes.Buffer
	v.Eval(&buf, ev)
	for _, restore := range restores {
		restore()
	}
	Log("call %q return %q", f.args[1], buf.Bytes())
	w.Write(buf.Bytes())
}

// http://www.gnu.org/software/make/manual/make.html#Value-Function
type funcValue struct{ fclosure }

func (f *funcValue) Arity() int { return 1 }
func (f *funcValue) Eval(w io.Writer, ev *Evaluator) {
	assertArity("value", 1, len(f.args))
	v := ev.LookupVar(f.args[1].String())
	w.Write([]byte(v.String()))
}

// http://www.gnu.org/software/make/manual/make.html#Eval-Function
type funcEval struct{ fclosure }

func (f *funcEval) Arity() int { return 1 }
func (f *funcEval) Eval(w io.Writer, ev *Evaluator) {
	assertArity("eval", 1, len(f.args))
	s := ev.Value(f.args[1])
	mk, err := ParseMakefileBytes(s, ev.filename, ev.lineno)
	if err != nil {
		panic(err)
	}

	for _, stmt := range mk.stmts {
		ev.eval(stmt)
	}
}

func (f *funcEval) Compact() Value {
	if len(f.args)-1 < 1 {
		return f
	}
	switch f.args[1].(type) {
	case literal, tmpval:
	default:
		// TODO(ukai): eval -> varassign. e.g. $(eval foo := $(x))
		return f
	}
	arg := f.args[1].String()
	arg = stripComment(arg)
	if arg == "" {
		return &funcNop{expr: f.String()}
	}
	f.args[1] = literal(arg)
	eq := strings.Index(arg, "=")
	if eq >= 0 {
		// TODO(ukai): factor out parse assign?
		lhs := arg[:eq]
		op := arg[eq : eq+1]
		if eq >= 1 && (arg[eq-1] == ':' || arg[eq-1] == '+' || arg[eq-1] == '?') {
			lhs = arg[:eq-1]
			op = arg[eq-1 : eq+1]
		}
		lhs = strings.TrimSpace(lhs)
		// no $... in rhs too.
		rhs := literal(strings.TrimLeft(arg[eq+1:], " \t"))
		return &funcEvalAssign{
			lhs: lhs,
			op:  op,
			rhs: rhs,
		}
	}
	return f
}

func stripComment(arg string) string {
	for {
		i := strings.Index(arg, "#")
		if i < 0 {
			return arg
		}
		eol := strings.Index(arg[i:], "\n")
		if eol < 0 {
			return arg[:i]
		}
		arg = arg[:i] + arg[eol+1:]
	}
}

type funcNop struct{ expr string }

func (f *funcNop) String() string             { return f.expr }
func (f *funcNop) Eval(io.Writer, *Evaluator) {}

type funcEvalAssign struct {
	lhs string
	op  string
	rhs Value
}

func (f *funcEvalAssign) String() string {
	return fmt.Sprintf("$(eval %s %s %s)", f.lhs, f.op, f.rhs)
}

func (f *funcEvalAssign) Eval(w io.Writer, ev *Evaluator) {
	rhs := ev.Value(f.rhs)
	var rvalue Var
	switch f.op {
	case ":=":
		rvalue = SimpleVar{value: tmpval(rhs), origin: "file"}
	case "=":
		rvalue = RecursiveVar{expr: tmpval(rhs), origin: "file"}
	case "+=":
		prev := ev.LookupVar(f.lhs)
		if !prev.IsDefined() {
			rvalue = prev.Append(ev, string(rhs))
		} else {
			rvalue = RecursiveVar{expr: tmpval(rhs), origin: "file"}
		}
	case "?=":
		prev := ev.LookupVar(f.lhs)
		if prev.IsDefined() {
			return
		}
		rvalue = RecursiveVar{expr: tmpval(rhs), origin: "file"}
	}
	ev.outVars.Assign(f.lhs, rvalue)
}

// http://www.gnu.org/software/make/manual/make.html#Origin-Function
type funcOrigin struct{ fclosure }

func (f *funcOrigin) Arity() int { return 1 }
func (f *funcOrigin) Eval(w io.Writer, ev *Evaluator) {
	assertArity("origin", 1, len(f.args))
	v := ev.LookupVar(f.args[1].String())
	w.Write([]byte(v.Origin()))
}

// https://www.gnu.org/software/make/manual/html_node/Flavor-Function.html#Flavor-Function
type funcFlavor struct{ fclosure }

func (f *funcFlavor) Arity() int { return 1 }
func (f *funcFlavor) Eval(w io.Writer, ev *Evaluator) {
	assertArity("flavor", 1, len(f.args))
	v := ev.LookupVar(f.args[1].String())
	w.Write([]byte(v.Flavor()))
}

// http://www.gnu.org/software/make/manual/make.html#Make-Control-Functions
type funcInfo struct{ fclosure }

func (f *funcInfo) Arity() int { return 1 }
func (f *funcInfo) Eval(w io.Writer, ev *Evaluator) {
	assertArity("info", 1, len(f.args))
	arg := ev.Value(f.args[1])
	fmt.Printf("%s\n", arg)
}

type funcWarning struct{ fclosure }

func (f *funcWarning) Arity() int { return 1 }
func (f *funcWarning) Eval(w io.Writer, ev *Evaluator) {
	assertArity("warning", 1, len(f.args))
	arg := ev.Value(f.args[1])
	fmt.Printf("%s:%d: %s\n", ev.filename, ev.lineno, arg)
}

type funcError struct{ fclosure }

func (f *funcError) Arity() int { return 1 }
func (f *funcError) Eval(w io.Writer, ev *Evaluator) {
	assertArity("error", 1, len(f.args))
	arg := ev.Value(f.args[1])
	Error(ev.filename, ev.lineno, "*** %s.", arg)
}

// http://www.gnu.org/software/make/manual/make.html#Foreach-Function
type funcForeach struct{ fclosure }

func (f *funcForeach) Arity() int { return 3 }

func (f *funcForeach) Eval(w io.Writer, ev *Evaluator) {
	assertArity("foreach", 3, len(f.args))
	varname := string(ev.Value(f.args[1]))
	list := ev.Values(f.args[2])
	text := f.args[3]
	restore := ev.outVars.save(varname)
	defer restore()
	space := false
	for _, word := range list {
		ev.outVars.Assign(varname,
			SimpleVar{
				value:  word,
				origin: "automatic",
			})
		if space {
			w.Write([]byte{' '})
		}
		w.Write(ev.Value(text))
		space = true
	}
}
