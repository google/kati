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

// mkFunc is a make function.
// http://www.gnu.org/software/make/manual/make.html#Functions

// mkFunc is make builtin function.
type mkFunc interface {
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
	funcMap = map[string]func() mkFunc{
		"patsubst":   func() mkFunc { return &funcPatsubst{} },
		"strip":      func() mkFunc { return &funcStrip{} },
		"subst":      func() mkFunc { return &funcSubst{} },
		"findstring": func() mkFunc { return &funcFindstring{} },
		"filter":     func() mkFunc { return &funcFilter{} },
		"filter-out": func() mkFunc { return &funcFilterOut{} },
		"sort":       func() mkFunc { return &funcSort{} },
		"word":       func() mkFunc { return &funcWord{} },
		"wordlist":   func() mkFunc { return &funcWordlist{} },
		"words":      func() mkFunc { return &funcWords{} },
		"firstword":  func() mkFunc { return &funcFirstword{} },
		"lastword":   func() mkFunc { return &funcLastword{} },

		"join":      func() mkFunc { return &funcJoin{} },
		"wildcard":  func() mkFunc { return &funcWildcard{} },
		"dir":       func() mkFunc { return &funcDir{} },
		"notdir":    func() mkFunc { return &funcNotdir{} },
		"suffix":    func() mkFunc { return &funcSuffix{} },
		"basename":  func() mkFunc { return &funcBasename{} },
		"addsuffix": func() mkFunc { return &funcAddsuffix{} },
		"addprefix": func() mkFunc { return &funcAddprefix{} },
		"realpath":  func() mkFunc { return &funcRealpath{} },
		"abspath":   func() mkFunc { return &funcAbspath{} },

		"if":  func() mkFunc { return &funcIf{} },
		"and": func() mkFunc { return &funcAnd{} },
		"or":  func() mkFunc { return &funcOr{} },

		"value": func() mkFunc { return &funcValue{} },

		"eval": func() mkFunc { return &funcEval{} },

		"shell":   func() mkFunc { return &funcShell{} },
		"call":    func() mkFunc { return &funcCall{} },
		"foreach": func() mkFunc { return &funcForeach{} },

		"origin":  func() mkFunc { return &funcOrigin{} },
		"flavor":  func() mkFunc { return &funcFlavor{} },
		"info":    func() mkFunc { return &funcInfo{} },
		"warning": func() mkFunc { return &funcWarning{} },
		"error":   func() mkFunc { return &funcError{} },
	}
)

type arityError struct {
	narg int
	name string
}

func (e arityError) Error() string {
	return fmt.Sprintf("*** insufficient number of arguments (%d) to function `%s'.", e.narg, e.name)
}

func assertArity(name string, req, n int) error {
	if n-1 < req {
		return arityError{narg: n - 1, name: name}
	}
	return nil
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
		return "$(func)"
	}
	arg0 := c.args[0].String()
	if arg0 == "" {
		return "$(func )"
	}
	cp := closeParen(arg0[0])
	if cp == 0 {
		return "${func }"
	}
	var args []string
	for _, arg := range c.args[1:] {
		args = append(args, arg.String())
	}
	return fmt.Sprintf("$%s %s%c", arg0, strings.Join(args, ","), cp)
}

func (c *fclosure) serialize() serializableVar {
	r := serializableVar{Type: "func"}
	for _, a := range c.args {
		r.Children = append(r.Children, a.serialize())
	}
	return r
}

func (c *fclosure) dump(d *dumpbuf) {
	d.Byte(valueTypeFunc)
	for _, a := range c.args {
		a.dump(d)
	}
}

// http://www.gnu.org/software/make/manual/make.html#Text-Functions
type funcSubst struct{ fclosure }

func (f *funcSubst) Arity() int { return 3 }
func (f *funcSubst) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("subst", 3, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	fargs, err := ev.args(abuf, f.args[1:]...)
	if err != nil {
		return err
	}
	t := time.Now()
	from := fargs[0]
	to := fargs[1]
	text := fargs[2]
	logf("subst from:%q to:%q text:%q", from, to, text)
	w.Write(bytes.Replace(text, from, to, -1))
	freeBuf(abuf)
	stats.add("funcbody", "subst", t)
	return nil
}

type funcPatsubst struct{ fclosure }

func (f *funcPatsubst) Arity() int { return 3 }
func (f *funcPatsubst) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("patsubst", 3, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	fargs, err := ev.args(abuf, f.args[1:]...)
	if err != nil {
		return err
	}
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
	return nil
}

type funcStrip struct{ fclosure }

func (f *funcStrip) Arity() int { return 1 }
func (f *funcStrip) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("strip", 1, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
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
	return nil
}

type funcFindstring struct{ fclosure }

func (f *funcFindstring) Arity() int { return 2 }
func (f *funcFindstring) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("findstring", 2, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	fargs, err := ev.args(abuf, f.args[1:]...)
	if err != nil {
		return err
	}
	t := time.Now()
	find := fargs[0]
	text := fargs[1]
	if bytes.Index(text, find) >= 0 {
		w.Write(find)
	}
	freeBuf(abuf)
	stats.add("funcbody", "findstring", t)
	return nil
}

type funcFilter struct{ fclosure }

func (f *funcFilter) Arity() int { return 2 }
func (f *funcFilter) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("filter", 2, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	fargs, err := ev.args(abuf, f.args[1:]...)
	if err != nil {
		return err
	}
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
	return nil
}

type funcFilterOut struct{ fclosure }

func (f *funcFilterOut) Arity() int { return 2 }
func (f *funcFilterOut) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("filter-out", 2, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	fargs, err := ev.args(abuf, f.args[1:]...)
	if err != nil {
		return err
	}
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
	return err
}

type funcSort struct{ fclosure }

func (f *funcSort) Arity() int { return 1 }
func (f *funcSort) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("sort", 1, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
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
	return nil
}

type funcWord struct{ fclosure }

func (f *funcWord) Arity() int { return 2 }
func (f *funcWord) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("word", 2, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	fargs, err := ev.args(abuf, f.args[1:]...)
	if err != nil {
		return err
	}
	t := time.Now()
	v := string(trimSpaceBytes(fargs[0]))
	index, ok := numericValueForFunc(v)
	if !ok {
		return ev.errorf(`*** non-numeric first argument to "word" function: %q.`, v)
	}
	if index == 0 {
		return ev.errorf(`*** first argument to "word" function must be greater than 0.`)
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
	return err
}

type funcWordlist struct{ fclosure }

func (f *funcWordlist) Arity() int { return 3 }
func (f *funcWordlist) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("wordlist", 3, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	fargs, err := ev.args(abuf, f.args[1:]...)
	if err != nil {
		return err
	}
	t := time.Now()
	v := string(trimSpaceBytes(fargs[0]))
	si, ok := numericValueForFunc(v)
	if !ok {
		return ev.errorf(`*** non-numeric first argument to "wordlist" function: %q.`, v)
	}
	if si == 0 {
		return ev.errorf(`*** invalid first argument to "wordlist" function: %s`, f.args[1])
	}
	v = string(trimSpaceBytes(fargs[1]))
	ei, ok := numericValueForFunc(v)
	if !ok {
		return ev.errorf(`*** non-numeric second argument to "wordlist" function: %q.`, v)
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
	return nil
}

type funcWords struct{ fclosure }

func (f *funcWords) Arity() int { return 1 }
func (f *funcWords) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("words", 1, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	t := time.Now()
	ws := newWordScanner(abuf.Bytes())
	n := 0
	for ws.Scan() {
		n++
	}
	freeBuf(abuf)
	io.WriteString(w, strconv.Itoa(n))
	stats.add("funcbody", "words", t)
	return nil
}

type funcFirstword struct{ fclosure }

func (f *funcFirstword) Arity() int { return 1 }
func (f *funcFirstword) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("firstword", 1, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	t := time.Now()
	ws := newWordScanner(abuf.Bytes())
	if ws.Scan() {
		w.Write(ws.Bytes())
	}
	freeBuf(abuf)
	stats.add("funcbody", "firstword", t)
	return nil
}

type funcLastword struct{ fclosure }

func (f *funcLastword) Arity() int { return 1 }
func (f *funcLastword) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("lastword", 1, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
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
	return err
}

// https://www.gnu.org/software/make/manual/html_node/File-Name-Functions.html#File-Name-Functions

type funcJoin struct{ fclosure }

func (f *funcJoin) Arity() int { return 2 }
func (f *funcJoin) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("join", 2, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	fargs, err := ev.args(abuf, f.args[1:]...)
	if err != nil {
		return err
	}
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
	return nil
}

type funcWildcard struct{ fclosure }

func (f *funcWildcard) Arity() int { return 1 }
func (f *funcWildcard) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("wildcard", 1, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	te := traceEvent.begin("wildcard", tmpval(abuf.Bytes()), traceEventMain)
	if ev.avoidIO && !UseWildcardCache {
		ev.hasIO = true
		io.WriteString(w, "$(/bin/ls -d ")
		w.Write(abuf.Bytes())
		io.WriteString(w, " 2> /dev/null)")
		traceEvent.end(te)
		freeBuf(abuf)
		return nil
	}
	t := time.Now()
	ws := newWordScanner(abuf.Bytes())
	sw := ssvWriter{w: w}
	for ws.Scan() {
		pat := string(ws.Bytes())
		err = wildcard(&sw, pat)
		if err != nil {
			return err
		}
	}
	traceEvent.end(te)
	freeBuf(abuf)
	stats.add("funcbody", "wildcard", t)
	return nil
}

type funcDir struct{ fclosure }

func (f *funcDir) Arity() int { return 1 }
func (f *funcDir) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("dir", 1, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	t := time.Now()
	ws := newWordScanner(abuf.Bytes())
	sw := ssvWriter{w: w}
	for ws.Scan() {
		name := filepath.Dir(string(string(ws.Bytes())))
		if name == "/" {
			sw.WriteString(name)
			continue
		}
		sw.WriteString(name + string(filepath.Separator))
	}
	freeBuf(abuf)
	stats.add("funcbody", "dir", t)
	return nil
}

type funcNotdir struct{ fclosure }

func (f *funcNotdir) Arity() int { return 1 }
func (f *funcNotdir) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("notdir", 1, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
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
	return nil
}

type funcSuffix struct{ fclosure }

func (f *funcSuffix) Arity() int { return 1 }
func (f *funcSuffix) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("suffix", 1, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
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
	return err
}

type funcBasename struct{ fclosure }

func (f *funcBasename) Arity() int { return 1 }
func (f *funcBasename) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("basename", 1, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
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
	return nil
}

type funcAddsuffix struct{ fclosure }

func (f *funcAddsuffix) Arity() int { return 2 }
func (f *funcAddsuffix) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("addsuffix", 2, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	fargs, err := ev.args(abuf, f.args[1:]...)
	if err != nil {
		return err
	}
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
	return err
}

type funcAddprefix struct{ fclosure }

func (f *funcAddprefix) Arity() int { return 2 }
func (f *funcAddprefix) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("addprefix", 2, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	fargs, err := ev.args(abuf, f.args[1:]...)
	if err != nil {
		return err
	}
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
	return err
}

type funcRealpath struct{ fclosure }

func (f *funcRealpath) Arity() int { return 1 }
func (f *funcRealpath) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("realpath", 1, len(f.args))
	if err != nil {
		return err
	}
	if ev.avoidIO {
		io.WriteString(w, "KATI_TODO(realpath)")
		ev.hasIO = true
		return nil
	}
	abuf := newBuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	t := time.Now()
	ws := newWordScanner(abuf.Bytes())
	sw := ssvWriter{w: w}
	for ws.Scan() {
		name := string(ws.Bytes())
		name, err := filepath.Abs(name)
		if err != nil {
			logf("abs: %v", err)
			continue
		}
		name, err = filepath.EvalSymlinks(name)
		if err != nil {
			logf("realpath: %v", err)
			continue
		}
		sw.WriteString(name)
	}
	freeBuf(abuf)
	stats.add("funcbody", "realpath", t)
	return err
}

type funcAbspath struct{ fclosure }

func (f *funcAbspath) Arity() int { return 1 }
func (f *funcAbspath) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("abspath", 1, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	t := time.Now()
	ws := newWordScanner(abuf.Bytes())
	sw := ssvWriter{w: w}
	for ws.Scan() {
		name := string(ws.Bytes())
		name, err := filepath.Abs(name)
		if err != nil {
			logf("abs: %v", err)
			continue
		}
		sw.WriteString(name)
	}
	freeBuf(abuf)
	stats.add("funcbody", "abspath", t)
	return nil
}

// http://www.gnu.org/software/make/manual/make.html#Conditional-Functions
type funcIf struct{ fclosure }

func (f *funcIf) Arity() int { return 3 }
func (f *funcIf) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("if", 2, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	if len(abuf.Bytes()) != 0 {
		freeBuf(abuf)
		return f.args[2].Eval(w, ev)
	}
	freeBuf(abuf)
	if len(f.args) > 3 {
		return f.args[3].Eval(w, ev)
	}
	return nil
}

type funcAnd struct{ fclosure }

func (f *funcAnd) Arity() int { return 0 }
func (f *funcAnd) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("and", 0, len(f.args))
	if err != nil {
		return nil
	}
	abuf := newBuf()
	var cond []byte
	for _, arg := range f.args[1:] {
		abuf.Reset()
		err = arg.Eval(abuf, ev)
		if err != nil {
			return err
		}
		cond = abuf.Bytes()
		if len(cond) == 0 {
			freeBuf(abuf)
			return nil
		}
	}
	w.Write(cond)
	freeBuf(abuf)
	return nil
}

type funcOr struct{ fclosure }

func (f *funcOr) Arity() int { return 0 }
func (f *funcOr) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("or", 0, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	for _, arg := range f.args[1:] {
		abuf.Reset()
		err = arg.Eval(abuf, ev)
		if err != nil {
			return err
		}
		cond := abuf.Bytes()
		if len(cond) != 0 {
			w.Write(cond)
			freeBuf(abuf)
			return nil
		}
	}
	freeBuf(abuf)
	return nil
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
	logf("has no IO - evaluate now: %s", s)
	return true
}

func (f *funcShell) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("shell", 1, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	if ev.avoidIO && !hasNoIoInShellScript(abuf.Bytes()) {
		te := traceEvent.begin("shell", tmpval(abuf.Bytes()), traceEventMain)
		ev.hasIO = true
		io.WriteString(w, "$(")
		w.Write(abuf.Bytes())
		writeByte(w, ')')
		traceEvent.end(te)
		freeBuf(abuf)
		return nil
	}
	arg := abuf.String()
	freeBuf(abuf)
	shellVar := ev.LookupVar("SHELL")
	// TODO: Should be Eval, not String.
	cmdline := []string{shellVar.String(), "-c", arg}
	if LogFlag {
		logf("shell %q", cmdline)
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
		logf("$(shell %q) failed: %q", arg, err)
	}
	w.Write(formatCommandOutput(out))
	traceEvent.end(te)
	return nil
}

func (f *funcShell) Compact() Value {
	if len(f.args)-1 < 1 {
		return f
	}
	if !UseFindCache && !UseShellBuiltins {
		return f
	}

	var exp expr
	switch v := f.args[1].(type) {
	case expr:
		exp = v
	default:
		exp = expr{v}
	}
	if UseShellBuiltins {
		// hack for android
		for _, sb := range shBuiltins {
			if v, ok := matchExpr(exp, sb.pattern); ok {
				logf("shell compact apply %s for %s", sb.name, exp)
				return sb.compact(f, v)
			}
		}
		logf("shell compact no match: %s", exp)
	}
	return f
}

// https://www.gnu.org/software/make/manual/html_node/Call-Function.html#Call-Function
type funcCall struct{ fclosure }

func (f *funcCall) Arity() int { return 0 }

func (f *funcCall) Eval(w io.Writer, ev *Evaluator) error {
	abuf := newBuf()
	fargs, err := ev.args(abuf, f.args[1:]...)
	if err != nil {
		return err
	}
	varname := fargs[0]
	variable := string(varname)
	te := traceEvent.begin("call", literal(variable), traceEventMain)
	if LogFlag {
		logf("call %q variable %q", f.args[1], variable)
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
			logf("call $%d: %q=>%q", i+1, arg, fargs[i+1])
		}
	}
	oldParams := ev.paramVars
	ev.paramVars = args

	var buf bytes.Buffer
	if LogFlag {
		w = io.MultiWriter(w, &buf)
	}
	err = v.Eval(w, ev)
	if err != nil {
		return err
	}
	ev.paramVars = oldParams
	traceEvent.end(te)
	if LogFlag {
		logf("call %q variable %q return %q", f.args[1], variable, buf.Bytes())
	}
	freeBuf(abuf)
	return nil
}

// http://www.gnu.org/software/make/manual/make.html#Value-Function
type funcValue struct{ fclosure }

func (f *funcValue) Arity() int { return 1 }
func (f *funcValue) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("value", 1, len(f.args))
	if err != nil {
		return err
	}
	v := ev.LookupVar(f.args[1].String())
	io.WriteString(w, v.String())
	return nil
}

// http://www.gnu.org/software/make/manual/make.html#Eval-Function
type funcEval struct{ fclosure }

func (f *funcEval) Arity() int { return 1 }
func (f *funcEval) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("eval", 1, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	s := abuf.Bytes()
	logf("eval %q at %s", s, ev.srcpos)
	mk, err := parseMakefileBytes(s, ev.srcpos)
	if err != nil {
		return ev.errorf("%v", err)
	}

	for _, stmt := range mk.stmts {
		err = ev.eval(stmt)
		if err != nil {
			return err
		}
	}
	freeBuf(abuf)
	return nil
}

func (f *funcEval) Compact() Value {
	if len(f.args)-1 < 1 {
		return f
	}
	switch arg := f.args[1].(type) {
	case literal, tmpval:
	case expr:
		if len(arg) == 1 {
			return f
		}
		switch prefix := arg[0].(type) {
		case literal, tmpval:
			lhs, op, rhsprefix, ok := parseAssignLiteral(prefix.String())
			if ok {
				// $(eval foo = $(bar))
				var rhs expr
				if rhsprefix != literal("") {
					rhs = append(rhs, rhsprefix)
				}
				rhs = append(rhs, arg[1:]...)
				logf("eval assign %#v => lhs:%q op:%q rhs:%#v", f, lhs, op, rhs)
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

func (f *funcNop) String() string                   { return f.expr }
func (f *funcNop) Eval(io.Writer, *Evaluator) error { return nil }
func (f *funcNop) serialize() serializableVar {
	return serializableVar{
		Type: "funcNop",
		V:    f.expr,
	}
}
func (f *funcNop) dump(d *dumpbuf) {
	d.Byte(valueTypeNop)
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

func (f *funcEvalAssign) Eval(w io.Writer, ev *Evaluator) error {
	var abuf bytes.Buffer
	err := f.rhs.Eval(&abuf, ev)
	if err != nil {
		return err
	}
	rhs := trimLeftSpaceBytes(abuf.Bytes())
	var rvalue Var
	switch f.op {
	case ":=":
		// TODO(ukai): compute parsed expr in Compact when f.rhs is
		// literal? e.g. literal("$(foo)") => varref{literal("foo")}.
		exp, _, err := parseExpr(rhs, nil, false)
		if err != nil {
			return ev.errorf("eval assign error: %q: %v", f.String(), err)
		}
		vbuf := newBuf()
		err = exp.Eval(vbuf, ev)
		if err != nil {
			return err
		}
		rvalue = &simpleVar{value: vbuf.String(), origin: "file"}
		freeBuf(vbuf)
	case "=":
		rvalue = &recursiveVar{expr: tmpval(rhs), origin: "file"}
	case "+=":
		prev := ev.LookupVar(f.lhs)
		if prev.IsDefined() {
			rvalue, err = prev.Append(ev, string(rhs))
			if err != nil {
				return err
			}
		} else {
			rvalue = &recursiveVar{expr: tmpval(rhs), origin: "file"}
		}
	case "?=":
		prev := ev.LookupVar(f.lhs)
		if prev.IsDefined() {
			return nil
		}
		rvalue = &recursiveVar{expr: tmpval(rhs), origin: "file"}
	}
	if LogFlag {
		logf("Eval ASSIGN: %s=%q (flavor:%q)", f.lhs, rvalue, rvalue.Flavor())
	}
	ev.outVars.Assign(f.lhs, rvalue)
	return nil
}

func (f *funcEvalAssign) serialize() serializableVar {
	return serializableVar{
		Type: "funcEvalAssign",
		Children: []serializableVar{
			serializableVar{V: f.lhs},
			serializableVar{V: f.op},
			f.rhs.serialize(),
		},
	}
}

func (f *funcEvalAssign) dump(d *dumpbuf) {
	d.Byte(valueTypeAssign)
	d.Str(f.lhs)
	d.Str(f.op)
	f.rhs.dump(d)
}

// http://www.gnu.org/software/make/manual/make.html#Origin-Function
type funcOrigin struct{ fclosure }

func (f *funcOrigin) Arity() int { return 1 }
func (f *funcOrigin) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("origin", 1, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	v := ev.LookupVar(abuf.String())
	freeBuf(abuf)
	io.WriteString(w, v.Origin())
	return nil
}

// https://www.gnu.org/software/make/manual/html_node/Flavor-Function.html#Flavor-Function
type funcFlavor struct{ fclosure }

func (f *funcFlavor) Arity() int { return 1 }
func (f *funcFlavor) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("flavor", 1, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	v := ev.LookupVar(abuf.String())
	freeBuf(abuf)
	io.WriteString(w, v.Flavor())
	return nil
}

// http://www.gnu.org/software/make/manual/make.html#Make-Control-Functions
type funcInfo struct{ fclosure }

func (f *funcInfo) Arity() int { return 1 }
func (f *funcInfo) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("info", 1, len(f.args))
	if err != nil {
		return err
	}
	if ev.avoidIO {
		io.WriteString(w, "KATI_TODO(info)")
		ev.hasIO = true
		return nil
	}
	abuf := newBuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", abuf.String())
	freeBuf(abuf)
	return nil
}

type funcWarning struct{ fclosure }

func (f *funcWarning) Arity() int { return 1 }
func (f *funcWarning) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("warning", 1, len(f.args))
	if err != nil {
		return err
	}
	if ev.avoidIO {
		io.WriteString(w, "KATI_TODO(warning)")
		ev.hasIO = true
		return nil
	}
	abuf := newBuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	fmt.Printf("%s: %s\n", ev.srcpos, abuf.String())
	freeBuf(abuf)
	return nil
}

type funcError struct{ fclosure }

func (f *funcError) Arity() int { return 1 }
func (f *funcError) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("error", 1, len(f.args))
	if err != nil {
		return err
	}
	if ev.avoidIO {
		io.WriteString(w, "KATI_TODO(error)")
		ev.hasIO = true
		return nil
	}
	var abuf buffer
	err = f.args[1].Eval(&abuf, ev)
	if err != nil {
		return err
	}
	return ev.errorf("*** %s.", abuf.String())
}

// http://www.gnu.org/software/make/manual/make.html#Foreach-Function
type funcForeach struct{ fclosure }

func (f *funcForeach) Arity() int { return 3 }

func (f *funcForeach) Eval(w io.Writer, ev *Evaluator) error {
	err := assertArity("foreach", 3, len(f.args))
	if err != nil {
		return err
	}
	abuf := newBuf()
	fargs, err := ev.args(abuf, f.args[1], f.args[2])
	if err != nil {
		return err
	}
	varname := string(fargs[0])
	ws := newWordScanner(fargs[1])
	text := f.args[3]
	restore := ev.outVars.save(varname)
	defer restore()
	space := false
	for ws.Scan() {
		word := ws.Bytes()
		ev.outVars.Assign(varname, &automaticVar{value: word})
		if space {
			writeByte(w, ' ')
		}
		err = text.Eval(w, ev)
		if err != nil {
			return err
		}
		space = true
	}
	freeBuf(abuf)
	return nil
}
