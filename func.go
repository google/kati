// Copyright 2015 Google Inc. All rights reserved
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kati

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
	"time"
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

func numericValueForFunc(v string) (int, bool) {
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return n, false
	}
	return n, true
}

func formatCommandOutput(out []byte) []byte {
	out = bytes.TrimRight(out, "\n")
	out = bytes.Replace(out, []byte{'\n'}, []byte{' '}, -1)
	return out
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

func (c *fclosure) Serialize() SerializableVar {
	r := SerializableVar{Type: "func"}
	for _, a := range c.args {
		r.Children = append(r.Children, a.Serialize())
	}
	return r
}

func (c *fclosure) Dump(w io.Writer) {
	dumpByte(w, valueTypeFunc)
	for _, a := range c.args {
		a.Dump(w)
	}
}

// http://www.gnu.org/software/make/manual/make.html#Text-Functions
type funcSubst struct{ fclosure }

func (f *funcSubst) Arity() int { return 3 }
func (f *funcSubst) Eval(w io.Writer, ev *Evaluator) {
	assertArity("subst", 3, len(f.args))
	abuf := newBuf()
	fargs := ev.args(abuf, f.args[1:]...)
	t := time.Now()
	from := fargs[0]
	to := fargs[1]
	text := fargs[2]
	Logf("subst from:%q to:%q text:%q", from, to, text)
	w.Write(bytes.Replace(text, from, to, -1))
	freeBuf(abuf)
	stats.add("funcbody", "subst", t)
}

type funcPatsubst struct{ fclosure }

func (f *funcPatsubst) Arity() int { return 3 }
func (f *funcPatsubst) Eval(w io.Writer, ev *Evaluator) {
	assertArity("patsubst", 3, len(f.args))
	abuf := newBuf()
	fargs := ev.args(abuf, f.args[1:]...)
	t := time.Now()
	pat := fargs[0]
	repl := fargs[1]
	ws := newWordScanner(fargs[2])
	space := false
	for ws.Scan() {
		if space {
			writeByte(w, ' ')
		}
		pre, subst, post := substPatternBytes(pat, repl, ws.Bytes())
		w.Write(pre)
		if subst != nil {
			w.Write(subst)
			w.Write(post)
		}
		space = true
	}
	freeBuf(abuf)
	stats.add("funcbody", "patsubst", t)
}

type funcStrip struct{ fclosure }

func (f *funcStrip) Arity() int { return 1 }
func (f *funcStrip) Eval(w io.Writer, ev *Evaluator) {
	assertArity("strip", 1, len(f.args))
	abuf := newBuf()
	f.args[1].Eval(abuf, ev)
	t := time.Now()
	ws := newWordScanner(abuf.Bytes())
	space := false
	for ws.Scan() {
		if space {
			writeByte(w, ' ')
		}
		w.Write(ws.Bytes())
		space = true
	}
	freeBuf(abuf)
	stats.add("funcbody", "strip", t)
}

type funcFindstring struct{ fclosure }

func (f *funcFindstring) Arity() int { return 2 }
func (f *funcFindstring) Eval(w io.Writer, ev *Evaluator) {
	assertArity("findstring", 2, len(f.args))
	abuf := newBuf()
	fargs := ev.args(abuf, f.args[1:]...)
	t := time.Now()
	find := fargs[0]
	text := fargs[1]
	if bytes.Index(text, find) >= 0 {
		w.Write(find)
	}
	freeBuf(abuf)
	stats.add("funcbody", "findstring", t)
}

type funcFilter struct{ fclosure }

func (f *funcFilter) Arity() int { return 2 }
func (f *funcFilter) Eval(w io.Writer, ev *Evaluator) {
	assertArity("filter", 2, len(f.args))
	abuf := newBuf()
	fargs := ev.args(abuf, f.args[1:]...)
	t := time.Now()
	var patterns [][]byte
	ws := newWordScanner(fargs[0])
	for ws.Scan() {
		patterns = append(patterns, ws.Bytes())
	}
	ws = newWordScanner(fargs[1])
	sw := ssvWriter{w: w}
	for ws.Scan() {
		text := ws.Bytes()
		for _, pat := range patterns {
			if matchPatternBytes(pat, text) {
				sw.Write(text)
			}
		}
	}
	freeBuf(abuf)
	stats.add("funcbody", "filter", t)
}

type funcFilterOut struct{ fclosure }

func (f *funcFilterOut) Arity() int { return 2 }
func (f *funcFilterOut) Eval(w io.Writer, ev *Evaluator) {
	assertArity("filter-out", 2, len(f.args))
	abuf := newBuf()
	fargs := ev.args(abuf, f.args[1:]...)
	t := time.Now()
	var patterns [][]byte
	ws := newWordScanner(fargs[0])
	for ws.Scan() {
		patterns = append(patterns, ws.Bytes())
	}
	ws = newWordScanner(fargs[1])
	sw := ssvWriter{w: w}
Loop:
	for ws.Scan() {
		text := ws.Bytes()
		for _, pat := range patterns {
			if matchPatternBytes(pat, text) {
				continue Loop
			}
		}
		sw.Write(text)
	}
	freeBuf(abuf)
	stats.add("funcbody", "filter-out", t)
}

type funcSort struct{ fclosure }

func (f *funcSort) Arity() int { return 1 }
func (f *funcSort) Eval(w io.Writer, ev *Evaluator) {
	assertArity("sort", 1, len(f.args))
	abuf := newBuf()
	f.args[1].Eval(abuf, ev)
	t := time.Now()
	ws := newWordScanner(abuf.Bytes())
	var toks []string
	for ws.Scan() {
		toks = append(toks, string(ws.Bytes()))
	}
	freeBuf(abuf)
	sort.Strings(toks)

	// Remove duplicate words.
	var prev string
	for _, tok := range toks {
		if prev == tok {
			continue
		}
		if prev != "" {
			writeByte(w, ' ')
		}
		io.WriteString(w, tok)
		prev = tok
	}
	stats.add("funcbody", "sort", t)
}

type funcWord struct{ fclosure }

func (f *funcWord) Arity() int { return 2 }
func (f *funcWord) Eval(w io.Writer, ev *Evaluator) {
	assertArity("word", 2, len(f.args))
	abuf := newBuf()
	fargs := ev.args(abuf, f.args[1:]...)
	t := time.Now()
	v := string(trimSpaceBytes(fargs[0]))
	index, ok := numericValueForFunc(v)
	if !ok {
		Error(ev.filename, ev.lineno, `*** non-numeric first argument to "word" function: %q.`, v)
	}
	if index == 0 {
		Error(ev.filename, ev.lineno, `*** first argument to "word" function must be greater than 0.`)
	}
	ws := newWordScanner(fargs[1])
	for ws.Scan() {
		index--
		if index == 0 {
			w.Write(ws.Bytes())
			break
		}
	}
	freeBuf(abuf)
	stats.add("funcbody", "word", t)
}

type funcWordlist struct{ fclosure }

func (f *funcWordlist) Arity() int { return 3 }
func (f *funcWordlist) Eval(w io.Writer, ev *Evaluator) {
	assertArity("wordlist", 3, len(f.args))
	abuf := newBuf()
	fargs := ev.args(abuf, f.args[1:]...)
	t := time.Now()
	v := string(trimSpaceBytes(fargs[0]))
	si, ok := numericValueForFunc(v)
	if !ok {
		Error(ev.filename, ev.lineno, `*** non-numeric first argument to "wordlist" function: %q.`, v)
	}
	if si == 0 {
		Error(ev.filename, ev.lineno, `*** invalid first argument to "wordlist" function: %s`, f.args[1])
	}
	v = string(trimSpaceBytes(fargs[1]))
	ei, ok := numericValueForFunc(v)
	if !ok {
		Error(ev.filename, ev.lineno, `*** non-numeric second argument to "wordlist" function: %q.`, v)
	}

	ws := newWordScanner(fargs[2])
	i := 0
	sw := ssvWriter{w: w}
	for ws.Scan() {
		i++
		if si <= i && i <= ei {
			sw.Write(ws.Bytes())
		}
	}
	freeBuf(abuf)
	stats.add("funcbody", "wordlist", t)
}

type funcWords struct{ fclosure }

func (f *funcWords) Arity() int { return 1 }
func (f *funcWords) Eval(w io.Writer, ev *Evaluator) {
	assertArity("words", 1, len(f.args))
	abuf := newBuf()
	f.args[1].Eval(abuf, ev)
	t := time.Now()
	ws := newWordScanner(abuf.Bytes())
	n := 0
	for ws.Scan() {
		n++
	}
	freeBuf(abuf)
	io.WriteString(w, strconv.Itoa(n))
	stats.add("funcbody", "words", t)
}

type funcFirstword struct{ fclosure }

func (f *funcFirstword) Arity() int { return 1 }
func (f *funcFirstword) Eval(w io.Writer, ev *Evaluator) {
	assertArity("firstword", 1, len(f.args))
	abuf := newBuf()
	f.args[1].Eval(abuf, ev)
	t := time.Now()
	ws := newWordScanner(abuf.Bytes())
	if ws.Scan() {
		w.Write(ws.Bytes())
	}
	freeBuf(abuf)
	stats.add("funcbody", "firstword", t)
}

type funcLastword struct{ fclosure }

func (f *funcLastword) Arity() int { return 1 }
func (f *funcLastword) Eval(w io.Writer, ev *Evaluator) {
	assertArity("lastword", 1, len(f.args))
	abuf := newBuf()
	f.args[1].Eval(abuf, ev)
	t := time.Now()
	ws := newWordScanner(abuf.Bytes())
	var lw []byte
	for ws.Scan() {
		lw = ws.Bytes()
	}
	if lw != nil {
		w.Write(lw)
	}
	freeBuf(abuf)
	stats.add("funcbody", "lastword", t)
}

// https://www.gnu.org/software/make/manual/html_node/File-Name-Functions.html#File-Name-Functions

type funcJoin struct{ fclosure }

func (f *funcJoin) Arity() int { return 2 }
func (f *funcJoin) Eval(w io.Writer, ev *Evaluator) {
	assertArity("join", 2, len(f.args))
	abuf := newBuf()
	fargs := ev.args(abuf, f.args[1:]...)
	t := time.Now()
	ws1 := newWordScanner(fargs[0])
	ws2 := newWordScanner(fargs[1])
	sw := ssvWriter{w: w}
	for {
		if w1, w2 := ws1.Scan(), ws2.Scan(); !w1 && !w2 {
			break
		}
		sw.Write(ws1.Bytes())
		// Use |w| not to append extra ' '.
		w.Write(ws2.Bytes())
	}
	freeBuf(abuf)
	stats.add("funcbody", "join", t)
}

type funcWildcard struct{ fclosure }

func (f *funcWildcard) Arity() int { return 1 }
func (f *funcWildcard) Eval(w io.Writer, ev *Evaluator) {
	assertArity("wildcard", 1, len(f.args))
	abuf := newBuf()
	f.args[1].Eval(abuf, ev)
	te := traceEvent.begin("wildcard", tmpval(abuf.Bytes()), traceEventMain)
	if ev.avoidIO && !UseWildcardCache {
		ev.hasIO = true
		io.WriteString(w, "$(/bin/ls -d ")
		w.Write(abuf.Bytes())
		io.WriteString(w, " 2> /dev/null)")
		traceEvent.end(te)
		freeBuf(abuf)
		return
	}
	t := time.Now()
	ws := newWordScanner(abuf.Bytes())
	sw := ssvWriter{w: w}
	for ws.Scan() {
		pat := string(ws.Bytes())
		wildcard(&sw, pat)
	}
	traceEvent.end(te)
	freeBuf(abuf)
	stats.add("funcbody", "wildcard", t)
}

type funcDir struct{ fclosure }

func (f *funcDir) Arity() int { return 1 }
func (f *funcDir) Eval(w io.Writer, ev *Evaluator) {
	assertArity("dir", 1, len(f.args))
	abuf := newBuf()
	f.args[1].Eval(abuf, ev)
	t := time.Now()
	ws := newWordScanner(abuf.Bytes())
	sw := ssvWriter{w: w}
	for ws.Scan() {
		name := string(ws.Bytes())
		if name == "/" {
			sw.WriteString(name)
			continue
		}
		sw.WriteString(filepath.Dir(string(name)) + string(filepath.Separator))
	}
	freeBuf(abuf)
	stats.add("funcbody", "dir", t)
}

type funcNotdir struct{ fclosure }

func (f *funcNotdir) Arity() int { return 1 }
func (f *funcNotdir) Eval(w io.Writer, ev *Evaluator) {
	assertArity("notdir", 1, len(f.args))
	abuf := newBuf()
	f.args[1].Eval(abuf, ev)
	t := time.Now()
	ws := newWordScanner(abuf.Bytes())
	sw := ssvWriter{w: w}
	for ws.Scan() {
		name := string(ws.Bytes())
		if name == string(filepath.Separator) {
			sw.Write([]byte{}) // separator
			continue
		}
		sw.WriteString(filepath.Base(name))
	}
	freeBuf(abuf)
	stats.add("funcbody", "notdir", t)
}

type funcSuffix struct{ fclosure }

func (f *funcSuffix) Arity() int { return 1 }
func (f *funcSuffix) Eval(w io.Writer, ev *Evaluator) {
	assertArity("suffix", 1, len(f.args))
	abuf := newBuf()
	f.args[1].Eval(abuf, ev)
	t := time.Now()
	ws := newWordScanner(abuf.Bytes())
	sw := ssvWriter{w: w}
	for ws.Scan() {
		tok := string(ws.Bytes())
		e := filepath.Ext(tok)
		if len(e) > 0 {
			sw.WriteString(e)
		}
	}
	freeBuf(abuf)
	stats.add("funcbody", "suffix", t)
}

type funcBasename struct{ fclosure }

func (f *funcBasename) Arity() int { return 1 }
func (f *funcBasename) Eval(w io.Writer, ev *Evaluator) {
	assertArity("basename", 1, len(f.args))
	abuf := newBuf()
	f.args[1].Eval(abuf, ev)
	t := time.Now()
	ws := newWordScanner(abuf.Bytes())
	sw := ssvWriter{w: w}
	for ws.Scan() {
		tok := string(ws.Bytes())
		e := stripExt(tok)
		sw.WriteString(e)
	}
	freeBuf(abuf)
	stats.add("funcbody", "basename", t)
}

type funcAddsuffix struct{ fclosure }

func (f *funcAddsuffix) Arity() int { return 2 }
func (f *funcAddsuffix) Eval(w io.Writer, ev *Evaluator) {
	assertArity("addsuffix", 2, len(f.args))
	abuf := newBuf()
	fargs := ev.args(abuf, f.args[1:]...)
	t := time.Now()
	suf := fargs[0]
	ws := newWordScanner(fargs[1])
	sw := ssvWriter{w: w}
	for ws.Scan() {
		sw.Write(ws.Bytes())
		// Use |w| not to append extra ' '.
		w.Write(suf)
	}
	freeBuf(abuf)
	stats.add("funcbody", "addsuffix", t)
}

type funcAddprefix struct{ fclosure }

func (f *funcAddprefix) Arity() int { return 2 }
func (f *funcAddprefix) Eval(w io.Writer, ev *Evaluator) {
	assertArity("addprefix", 2, len(f.args))
	abuf := newBuf()
	fargs := ev.args(abuf, f.args[1:]...)
	t := time.Now()
	pre := fargs[0]
	ws := newWordScanner(fargs[1])
	sw := ssvWriter{w: w}
	for ws.Scan() {
		sw.Write(pre)
		// Use |w| not to append extra ' '.
		w.Write(ws.Bytes())
	}
	freeBuf(abuf)
	stats.add("funcbody", "addprefix", t)
}

type funcRealpath struct{ fclosure }

func (f *funcRealpath) Arity() int { return 1 }
func (f *funcRealpath) Eval(w io.Writer, ev *Evaluator) {
	assertArity("realpath", 1, len(f.args))
	if ev.avoidIO {
		io.WriteString(w, "KATI_TODO(realpath)")
		ev.hasIO = true
		return
	}
	abuf := newBuf()
	f.args[1].Eval(abuf, ev)
	t := time.Now()
	ws := newWordScanner(abuf.Bytes())
	sw := ssvWriter{w: w}
	for ws.Scan() {
		name := string(ws.Bytes())
		name, err := filepath.Abs(name)
		if err != nil {
			Logf("abs: %v", err)
			continue
		}
		name, err = filepath.EvalSymlinks(name)
		if err != nil {
			Logf("realpath: %v", err)
			continue
		}
		sw.WriteString(name)
	}
	freeBuf(abuf)
	stats.add("funcbody", "realpath", t)
}

type funcAbspath struct{ fclosure }

func (f *funcAbspath) Arity() int { return 1 }
func (f *funcAbspath) Eval(w io.Writer, ev *Evaluator) {
	assertArity("abspath", 1, len(f.args))
	abuf := newBuf()
	f.args[1].Eval(abuf, ev)
	t := time.Now()
	ws := newWordScanner(abuf.Bytes())
	sw := ssvWriter{w: w}
	for ws.Scan() {
		name := string(ws.Bytes())
		name, err := filepath.Abs(name)
		if err != nil {
			Logf("abs: %v", err)
			continue
		}
		sw.WriteString(name)
	}
	freeBuf(abuf)
	stats.add("funcbody", "abspath", t)
}

// http://www.gnu.org/software/make/manual/make.html#Conditional-Functions
type funcIf struct{ fclosure }

func (f *funcIf) Arity() int { return 3 }
func (f *funcIf) Eval(w io.Writer, ev *Evaluator) {
	assertArity("if", 2, len(f.args))
	abuf := newBuf()
	f.args[1].Eval(abuf, ev)
	if len(abuf.Bytes()) != 0 {
		freeBuf(abuf)
		f.args[2].Eval(w, ev)
		return
	}
	freeBuf(abuf)
	if len(f.args) > 3 {
		f.args[3].Eval(w, ev)
	}
}

type funcAnd struct{ fclosure }

func (f *funcAnd) Arity() int { return 0 }
func (f *funcAnd) Eval(w io.Writer, ev *Evaluator) {
	assertArity("and", 0, len(f.args))
	abuf := newBuf()
	var cond []byte
	for _, arg := range f.args[1:] {
		abuf.Reset()
		arg.Eval(abuf, ev)
		cond = abuf.Bytes()
		if len(cond) == 0 {
			freeBuf(abuf)
			return
		}
	}
	w.Write(cond)
	freeBuf(abuf)
}

type funcOr struct{ fclosure }

func (f *funcOr) Arity() int { return 0 }
func (f *funcOr) Eval(w io.Writer, ev *Evaluator) {
	assertArity("or", 0, len(f.args))
	abuf := newBuf()
	for _, arg := range f.args[1:] {
		abuf.Reset()
		arg.Eval(abuf, ev)
		cond := abuf.Bytes()
		if len(cond) != 0 {
			w.Write(cond)
			freeBuf(abuf)
			return
		}
	}
	freeBuf(abuf)
}

// http://www.gnu.org/software/make/manual/make.html#Shell-Function
type funcShell struct{ fclosure }

func (f *funcShell) Arity() int { return 1 }

// A hack for Android build. We need to evaluate things like $((3+4))
// when we emit ninja file, because the result of such expressions
// will be passed to other make functions.
// TODO: Maybe we should modify Android's Makefile and remove this
// workaround. It would be also nice if we can detect things like
// this.
func hasNoIoInShellScript(s []byte) bool {
	if len(s) == 0 {
		return true
	}
	if !bytes.HasPrefix(s, []byte("echo $((")) || s[len(s)-1] != ')' {
		return false
	}
	Logf("has no IO - evaluate now: %s", s)
	return true
}

func (f *funcShell) Eval(w io.Writer, ev *Evaluator) {
	assertArity("shell", 1, len(f.args))
	abuf := newBuf()
	f.args[1].Eval(abuf, ev)
	if ev.avoidIO && !hasNoIoInShellScript(abuf.Bytes()) {
		te := traceEvent.begin("shell", tmpval(abuf.Bytes()), traceEventMain)
		ev.hasIO = true
		io.WriteString(w, "$(")
		w.Write(abuf.Bytes())
		writeByte(w, ')')
		traceEvent.end(te)
		freeBuf(abuf)
		return
	}
	arg := abuf.String()
	freeBuf(abuf)
	shellVar := ev.LookupVar("SHELL")
	// TODO: Should be Eval, not String.
	cmdline := []string{shellVar.String(), "-c", arg}
	if LogFlag {
		Logf("shell %q", cmdline)
	}
	cmd := exec.Cmd{
		Path:   cmdline[0],
		Args:   cmdline,
		Stderr: os.Stderr,
	}
	te := traceEvent.begin("shell", literal(arg), traceEventMain)
	out, err := cmd.Output()
	shellStats.add(time.Since(te.t))
	if err != nil {
		Logf("$(shell %q) failed: %q", arg, err)
	}
	w.Write(formatCommandOutput(out))
	traceEvent.end(te)
}

func (f *funcShell) Compact() Value {
	if len(f.args)-1 < 1 {
		return f
	}
	if !UseFindCache && !UseShellBuiltins {
		return f
	}

	var expr Expr
	switch v := f.args[1].(type) {
	case Expr:
		expr = v
	default:
		expr = Expr{v}
	}
	if UseShellBuiltins {
		// hack for android
		for _, sb := range shBuiltins {
			if v, ok := matchExpr(expr, sb.pattern); ok {
				Logf("shell compact apply %s for %s", sb.name, expr)
				return sb.compact(f, v)
			}
		}
		Logf("shell compact no match: %s", expr)
	}
	return f
}

// https://www.gnu.org/software/make/manual/html_node/Call-Function.html#Call-Function
type funcCall struct{ fclosure }

func (f *funcCall) Arity() int { return 0 }

func (f *funcCall) Eval(w io.Writer, ev *Evaluator) {
	abuf := newBuf()
	fargs := ev.args(abuf, f.args[1:]...)
	varname := fargs[0]
	variable := string(varname)
	te := traceEvent.begin("call", literal(variable), traceEventMain)
	if LogFlag {
		Logf("call %q variable %q", f.args[1], variable)
	}
	v := ev.LookupVar(variable)
	// Evalualte all arguments first before we modify the table.
	var args []tmpval
	// $0 is variable.
	args = append(args, tmpval(varname))
	// TODO(ukai): If variable is the name of a built-in function,
	// the built-in function is always invoked (even if a make variable
	// by that name also exists).

	for i, arg := range fargs[1:] {
		// f.args[2]=>args[1] will be $1.
		args = append(args, tmpval(arg))
		if LogFlag {
			Logf("call $%d: %q=>%q", i+1, arg, fargs[i+1])
		}
	}
	oldParams := ev.paramVars
	ev.paramVars = args

	var buf bytes.Buffer
	if LogFlag {
		w = io.MultiWriter(w, &buf)
	}
	v.Eval(w, ev)
	ev.paramVars = oldParams
	traceEvent.end(te)
	if LogFlag {
		Logf("call %q variable %q return %q", f.args[1], variable, buf.Bytes())
	}
	freeBuf(abuf)
}

// http://www.gnu.org/software/make/manual/make.html#Value-Function
type funcValue struct{ fclosure }

func (f *funcValue) Arity() int { return 1 }
func (f *funcValue) Eval(w io.Writer, ev *Evaluator) {
	assertArity("value", 1, len(f.args))
	v := ev.LookupVar(f.args[1].String())
	io.WriteString(w, v.String())
}

// http://www.gnu.org/software/make/manual/make.html#Eval-Function
type funcEval struct{ fclosure }

func (f *funcEval) Arity() int { return 1 }
func (f *funcEval) Eval(w io.Writer, ev *Evaluator) {
	assertArity("eval", 1, len(f.args))
	abuf := newBuf()
	f.args[1].Eval(abuf, ev)
	s := abuf.Bytes()
	Logf("eval %q at %s:%d", s, ev.filename, ev.lineno)
	mk, err := ParseMakefileBytes(s, ev.filename, ev.lineno)
	if err != nil {
		panic(err)
	}

	for _, stmt := range mk.stmts {
		ev.eval(stmt)
	}
	freeBuf(abuf)
}

func (f *funcEval) Compact() Value {
	if len(f.args)-1 < 1 {
		return f
	}
	switch arg := f.args[1].(type) {
	case literal, tmpval:
	case Expr:
		if len(arg) == 1 {
			return f
		}
		switch prefix := arg[0].(type) {
		case literal, tmpval:
			lhs, op, rhsprefix, ok := parseAssignLiteral(prefix.String())
			if ok {
				// $(eval foo = $(bar))
				var rhs Expr
				if rhsprefix != literal("") {
					rhs = append(rhs, rhsprefix)
				}
				rhs = append(rhs, arg[1:]...)
				Logf("eval assign %#v => lhs:%q op:%q rhs:%#v", f, lhs, op, rhs)
				return &funcEvalAssign{
					lhs: lhs,
					op:  op,
					rhs: compactExpr(rhs),
				}
			}
		}
		// TODO(ukai): eval -> varassign. e.g $(eval $(foo) := $(x)).
		return f
	default:
		return f
	}
	arg := f.args[1].String()
	arg = stripComment(arg)
	if arg == "" || strings.TrimSpace(arg) == "" {
		return &funcNop{expr: f.String()}
	}
	f.args[1] = literal(arg)
	lhs, op, rhs, ok := parseAssignLiteral(f.args[1].String())
	if ok {
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
func (f *funcNop) Serialize() SerializableVar {
	return SerializableVar{
		Type: "funcNop",
		V:    f.expr,
	}
}
func (f *funcNop) Dump(w io.Writer) {
	dumpByte(w, valueTypeNop)
}

func parseAssignLiteral(s string) (lhs, op string, rhs Value, ok bool) {
	eq := strings.Index(s, "=")
	if eq < 0 {
		return "", "", nil, false
	}
	// TODO(ukai): factor out parse assign?
	lhs = s[:eq]
	op = s[eq : eq+1]
	if eq >= 1 && (s[eq-1] == ':' || s[eq-1] == '+' || s[eq-1] == '?') {
		lhs = s[:eq-1]
		op = s[eq-1 : eq+1]
	}
	lhs = strings.TrimSpace(lhs)
	if strings.IndexAny(lhs, ":$") >= 0 {
		// target specific var, or need eval.
		return "", "", nil, false
	}
	r := strings.TrimLeft(s[eq+1:], " \t")
	rhs = literal(r)
	return lhs, op, rhs, true
}

type funcEvalAssign struct {
	lhs string
	op  string
	rhs Value
}

func (f *funcEvalAssign) String() string {
	return fmt.Sprintf("$(eval %s %s %s)", f.lhs, f.op, f.rhs)
}

func (f *funcEvalAssign) Eval(w io.Writer, ev *Evaluator) {
	var abuf bytes.Buffer
	f.rhs.Eval(&abuf, ev)
	rhs := trimLeftSpaceBytes(abuf.Bytes())
	var rvalue Var
	switch f.op {
	case ":=":
		// TODO(ukai): compute parsed expr in Compact when f.rhs is
		// literal? e.g. literal("$(foo)") => varref{literal("foo")}.
		expr, _, err := parseExpr(rhs, nil, false)
		if err != nil {
			panic(fmt.Sprintf("eval assign error: %q: %v", f.String(), err))
		}
		vbuf := newBuf()
		expr.Eval(vbuf, ev)
		rvalue = &SimpleVar{value: vbuf.String(), origin: "file"}
		freeBuf(vbuf)
	case "=":
		rvalue = &RecursiveVar{expr: tmpval(rhs), origin: "file"}
	case "+=":
		prev := ev.LookupVar(f.lhs)
		if prev.IsDefined() {
			rvalue = prev.Append(ev, string(rhs))
		} else {
			rvalue = &RecursiveVar{expr: tmpval(rhs), origin: "file"}
		}
	case "?=":
		prev := ev.LookupVar(f.lhs)
		if prev.IsDefined() {
			return
		}
		rvalue = &RecursiveVar{expr: tmpval(rhs), origin: "file"}
	}
	if LogFlag {
		Logf("Eval ASSIGN: %s=%q (flavor:%q)", f.lhs, rvalue, rvalue.Flavor())
	}
	ev.outVars.Assign(f.lhs, rvalue)
}

func (f *funcEvalAssign) Serialize() SerializableVar {
	return SerializableVar{
		Type: "funcEvalAssign",
		Children: []SerializableVar{
			SerializableVar{V: f.lhs},
			SerializableVar{V: f.op},
			f.rhs.Serialize(),
		},
	}
}

func (f *funcEvalAssign) Dump(w io.Writer) {
	dumpByte(w, valueTypeAssign)
	dumpString(w, f.lhs)
	dumpString(w, f.op)
	f.rhs.Dump(w)
}

// http://www.gnu.org/software/make/manual/make.html#Origin-Function
type funcOrigin struct{ fclosure }

func (f *funcOrigin) Arity() int { return 1 }
func (f *funcOrigin) Eval(w io.Writer, ev *Evaluator) {
	assertArity("origin", 1, len(f.args))
	v := ev.LookupVar(f.args[1].String())
	io.WriteString(w, v.Origin())
}

// https://www.gnu.org/software/make/manual/html_node/Flavor-Function.html#Flavor-Function
type funcFlavor struct{ fclosure }

func (f *funcFlavor) Arity() int { return 1 }
func (f *funcFlavor) Eval(w io.Writer, ev *Evaluator) {
	assertArity("flavor", 1, len(f.args))
	v := ev.LookupVar(f.args[1].String())
	io.WriteString(w, v.Flavor())
}

// http://www.gnu.org/software/make/manual/make.html#Make-Control-Functions
type funcInfo struct{ fclosure }

func (f *funcInfo) Arity() int { return 1 }
func (f *funcInfo) Eval(w io.Writer, ev *Evaluator) {
	assertArity("info", 1, len(f.args))
	if ev.avoidIO {
		io.WriteString(w, "KATI_TODO(info)")
		ev.hasIO = true
		return
	}
	abuf := newBuf()
	f.args[1].Eval(abuf, ev)
	fmt.Printf("%s\n", abuf.String())
	freeBuf(abuf)
}

type funcWarning struct{ fclosure }

func (f *funcWarning) Arity() int { return 1 }
func (f *funcWarning) Eval(w io.Writer, ev *Evaluator) {
	assertArity("warning", 1, len(f.args))
	if ev.avoidIO {
		io.WriteString(w, "KATI_TODO(warning)")
		ev.hasIO = true
		return
	}
	abuf := newBuf()
	f.args[1].Eval(abuf, ev)
	fmt.Printf("%s:%d: %s\n", ev.filename, ev.lineno, abuf.String())
	freeBuf(abuf)
}

type funcError struct{ fclosure }

func (f *funcError) Arity() int { return 1 }
func (f *funcError) Eval(w io.Writer, ev *Evaluator) {
	assertArity("error", 1, len(f.args))
	if ev.avoidIO {
		io.WriteString(w, "KATI_TODO(error)")
		ev.hasIO = true
		return
	}
	abuf := newBuf()
	f.args[1].Eval(abuf, ev)
	Error(ev.filename, ev.lineno, "*** %s.", abuf.String())
	freeBuf(abuf)
}

// http://www.gnu.org/software/make/manual/make.html#Foreach-Function
type funcForeach struct{ fclosure }

func (f *funcForeach) Arity() int { return 3 }

func (f *funcForeach) Eval(w io.Writer, ev *Evaluator) {
	assertArity("foreach", 3, len(f.args))
	abuf := newBuf()
	fargs := ev.args(abuf, f.args[1], f.args[2])
	varname := string(fargs[0])
	ws := newWordScanner(fargs[1])
	text := f.args[3]
	restore := ev.outVars.save(varname)
	defer restore()
	space := false
	for ws.Scan() {
		word := ws.Bytes()
		ev.outVars.Assign(varname, &AutomaticVar{value: word})
		if space {
			writeByte(w, ' ')
		}
		text.Eval(w, ev)
		space = true
	}
	freeBuf(abuf)
}
