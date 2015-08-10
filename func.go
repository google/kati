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

	"github.com/golang/glog"
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
func (f *funcSubst) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("subst", 3, len(f.args))
	if err != nil {
		return err
	}
	abuf := newEbuf()
	fargs, err := ev.args(abuf, f.args[1:]...)
	if err != nil {
		return err
	}
	t := time.Now()
	from := fargs[0]
	to := fargs[1]
	text := fargs[2]
	glog.V(1).Infof("subst from:%q to:%q text:%q", from, to, text)
	if len(from) == 0 {
		w.Write(text)
		w.Write(to)
	} else {
		w.Write(bytes.Replace(text, from, to, -1))
	}
	abuf.release()
	stats.add("funcbody", "subst", t)
	return nil
}

type funcPatsubst struct{ fclosure }

func (f *funcPatsubst) Arity() int { return 3 }
func (f *funcPatsubst) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("patsubst", 3, len(f.args))
	if err != nil {
		return err
	}
	abuf := newEbuf()
	fargs, err := ev.args(abuf, f.args[1], f.args[2])
	if err != nil {
		return err
	}
	wb := newWbuf()
	err = f.args[3].Eval(wb, ev)
	if err != nil {
		return err
	}
	t := time.Now()
	pat := fargs[0]
	repl := fargs[1]
	for _, word := range wb.words {
		pre, subst, post := substPatternBytes(pat, repl, word)
		var sword []byte
		sword = append(sword, pre...)
		if subst != nil {
			sword = append(sword, subst...)
			sword = append(sword, post...)
		}
		w.writeWord(sword)
	}
	abuf.release()
	wb.release()
	stats.add("funcbody", "patsubst", t)
	return nil
}

type funcStrip struct{ fclosure }

func (f *funcStrip) Arity() int { return 1 }
func (f *funcStrip) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("strip", 1, len(f.args))
	if err != nil {
		return err
	}
	wb := newWbuf()
	err = f.args[1].Eval(wb, ev)
	if err != nil {
		return err
	}
	t := time.Now()
	for _, word := range wb.words {
		w.writeWord(word)
	}
	wb.release()
	stats.add("funcbody", "strip", t)
	return nil
}

type funcFindstring struct{ fclosure }

func (f *funcFindstring) Arity() int { return 2 }
func (f *funcFindstring) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("findstring", 2, len(f.args))
	if err != nil {
		return err
	}
	abuf := newEbuf()
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
	abuf.release()
	stats.add("funcbody", "findstring", t)
	return nil
}

type funcFilter struct{ fclosure }

func (f *funcFilter) Arity() int { return 2 }
func (f *funcFilter) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("filter", 2, len(f.args))
	if err != nil {
		return err
	}
	patternsBuffer := newWbuf()
	err = f.args[1].Eval(patternsBuffer, ev)
	if err != nil {
		return err
	}
	textBuffer := newWbuf()
	err = f.args[2].Eval(textBuffer, ev)
	if err != nil {
		return err
	}
	t := time.Now()
	for _, text := range textBuffer.words {
		for _, pat := range patternsBuffer.words {
			if matchPatternBytes(pat, text) {
				w.writeWord(text)
			}
		}
	}
	patternsBuffer.release()
	textBuffer.release()
	stats.add("funcbody", "filter", t)
	return nil
}

type funcFilterOut struct{ fclosure }

func (f *funcFilterOut) Arity() int { return 2 }
func (f *funcFilterOut) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("filter-out", 2, len(f.args))
	if err != nil {
		return err
	}
	patternsBuffer := newWbuf()
	err = f.args[1].Eval(patternsBuffer, ev)
	if err != nil {
		return err
	}
	textBuffer := newWbuf()
	err = f.args[2].Eval(textBuffer, ev)
	if err != nil {
		return err
	}
	t := time.Now()
Loop:
	for _, text := range textBuffer.words {
		for _, pat := range patternsBuffer.words {
			if matchPatternBytes(pat, text) {
				continue Loop
			}
		}
		w.writeWord(text)
	}
	patternsBuffer.release()
	textBuffer.release()
	stats.add("funcbody", "filter-out", t)
	return err
}

type funcSort struct{ fclosure }

func (f *funcSort) Arity() int { return 1 }
func (f *funcSort) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("sort", 1, len(f.args))
	if err != nil {
		return err
	}
	wb := newWbuf()
	err = f.args[1].Eval(wb, ev)
	if err != nil {
		return err
	}
	t := time.Now()
	var toks []string
	for _, tok := range wb.words {
		toks = append(toks, string(tok))
	}
	wb.release()
	sort.Strings(toks)

	// Remove duplicate words.
	var prev string
	for _, tok := range toks {
		if prev == tok {
			continue
		}
		w.writeWordString(tok)
		prev = tok
	}
	stats.add("funcbody", "sort", t)
	return nil
}

type funcWord struct{ fclosure }

func (f *funcWord) Arity() int { return 2 }
func (f *funcWord) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("word", 2, len(f.args))
	if err != nil {
		return err
	}
	abuf := newEbuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	v := string(trimSpaceBytes(abuf.Bytes()))
	abuf.release()
	index, ok := numericValueForFunc(v)
	if !ok {
		return ev.errorf(`*** non-numeric first argument to "word" function: %q.`, v)
	}
	if index == 0 {
		return ev.errorf(`*** first argument to "word" function must be greater than 0.`)
	}
	wb := newWbuf()
	err = f.args[2].Eval(wb, ev)
	if err != nil {
		return err
	}
	t := time.Now()
	if index-1 < len(wb.words) {
		w.writeWord(wb.words[index-1])
	}
	wb.release()
	stats.add("funcbody", "word", t)
	return err
}

type funcWordlist struct{ fclosure }

func (f *funcWordlist) Arity() int { return 3 }
func (f *funcWordlist) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("wordlist", 3, len(f.args))
	if err != nil {
		return err
	}
	abuf := newEbuf()
	fargs, err := ev.args(abuf, f.args[1], f.args[2])
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
	abuf.release()

	wb := newWbuf()
	err = f.args[3].Eval(wb, ev)
	if err != nil {
		return err
	}
	for i, word := range wb.words {
		if si <= i+1 && i+1 <= ei {
			w.writeWord(word)
		}
	}
	wb.release()
	stats.add("funcbody", "wordlist", t)
	return nil
}

type funcWords struct{ fclosure }

func (f *funcWords) Arity() int { return 1 }
func (f *funcWords) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("words", 1, len(f.args))
	if err != nil {
		return err
	}
	wb := newWbuf()
	err = f.args[1].Eval(wb, ev)
	if err != nil {
		return err
	}
	t := time.Now()
	n := len(wb.words)
	wb.release()
	w.writeWordString(strconv.Itoa(n))
	stats.add("funcbody", "words", t)
	return nil
}

type funcFirstword struct{ fclosure }

func (f *funcFirstword) Arity() int { return 1 }
func (f *funcFirstword) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("firstword", 1, len(f.args))
	if err != nil {
		return err
	}
	wb := newWbuf()
	err = f.args[1].Eval(wb, ev)
	if err != nil {
		return err
	}
	t := time.Now()
	if len(wb.words) > 0 {
		w.writeWord(wb.words[0])
	}
	wb.release()
	stats.add("funcbody", "firstword", t)
	return nil
}

type funcLastword struct{ fclosure }

func (f *funcLastword) Arity() int { return 1 }
func (f *funcLastword) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("lastword", 1, len(f.args))
	if err != nil {
		return err
	}
	wb := newWbuf()
	err = f.args[1].Eval(wb, ev)
	if err != nil {
		return err
	}
	t := time.Now()
	if len(wb.words) > 0 {
		w.writeWord(wb.words[len(wb.words)-1])
	}
	wb.release()
	stats.add("funcbody", "lastword", t)
	return err
}

// https://www.gnu.org/software/make/manual/html_node/File-Name-Functions.html#File-Name-Functions

type funcJoin struct{ fclosure }

func (f *funcJoin) Arity() int { return 2 }
func (f *funcJoin) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("join", 2, len(f.args))
	if err != nil {
		return err
	}
	wb1 := newWbuf()
	err = f.args[1].Eval(wb1, ev)
	if err != nil {
		return err
	}
	wb2 := newWbuf()
	err = f.args[2].Eval(wb2, ev)
	if err != nil {
		return err
	}
	t := time.Now()
	for i := 0; i < len(wb1.words) || i < len(wb2.words); i++ {
		var word []byte
		if i < len(wb1.words) {
			word = append(word, wb1.words[i]...)
		}
		if i < len(wb2.words) {
			word = append(word, wb2.words[i]...)
		}
		w.writeWord(word)
	}
	wb1.release()
	wb2.release()
	stats.add("funcbody", "join", t)
	return nil
}

type funcWildcard struct{ fclosure }

func (f *funcWildcard) Arity() int { return 1 }
func (f *funcWildcard) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("wildcard", 1, len(f.args))
	if err != nil {
		return err
	}
	wb := newWbuf()
	err = f.args[1].Eval(wb, ev)
	if err != nil {
		return err
	}
	te := traceEvent.begin("wildcard", tmpval(wb.Bytes()), traceEventMain)
	// Note GNU make does not delay the execution of $(wildcard) so we
	// do not need to check avoid_io here.
	t := time.Now()
	for _, word := range wb.words {
		pat := string(word)
		err = wildcard(w, pat)
		if err != nil {
			return err
		}
	}
	wb.release()
	traceEvent.end(te)
	stats.add("funcbody", "wildcard", t)
	return nil
}

type funcDir struct{ fclosure }

func (f *funcDir) Arity() int { return 1 }
func (f *funcDir) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("dir", 1, len(f.args))
	if err != nil {
		return err
	}
	wb := newWbuf()
	err = f.args[1].Eval(wb, ev)
	if err != nil {
		return err
	}
	t := time.Now()
	for _, word := range wb.words {
		name := filepath.Dir(string(word))
		if name == "/" {
			w.writeWordString(name)
			continue
		}
		w.writeWordString(name + string(filepath.Separator))
	}
	wb.release()
	stats.add("funcbody", "dir", t)
	return nil
}

type funcNotdir struct{ fclosure }

func (f *funcNotdir) Arity() int { return 1 }
func (f *funcNotdir) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("notdir", 1, len(f.args))
	if err != nil {
		return err
	}
	wb := newWbuf()
	err = f.args[1].Eval(wb, ev)
	if err != nil {
		return err
	}
	t := time.Now()
	for _, word := range wb.words {
		name := string(word)
		if name == string(filepath.Separator) {
			w.writeWord([]byte{}) // separator
			continue
		}
		w.writeWordString(filepath.Base(name))
	}
	wb.release()
	stats.add("funcbody", "notdir", t)
	return nil
}

type funcSuffix struct{ fclosure }

func (f *funcSuffix) Arity() int { return 1 }
func (f *funcSuffix) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("suffix", 1, len(f.args))
	if err != nil {
		return err
	}
	wb := newWbuf()
	err = f.args[1].Eval(wb, ev)
	if err != nil {
		return err
	}
	t := time.Now()
	for _, word := range wb.words {
		tok := string(word)
		e := filepath.Ext(tok)
		if len(e) > 0 {
			w.writeWordString(e)
		}
	}
	wb.release()
	stats.add("funcbody", "suffix", t)
	return err
}

type funcBasename struct{ fclosure }

func (f *funcBasename) Arity() int { return 1 }
func (f *funcBasename) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("basename", 1, len(f.args))
	if err != nil {
		return err
	}
	wb := newWbuf()
	err = f.args[1].Eval(wb, ev)
	if err != nil {
		return err
	}
	t := time.Now()
	for _, word := range wb.words {
		tok := string(word)
		e := stripExt(tok)
		w.writeWordString(e)
	}
	wb.release()
	stats.add("funcbody", "basename", t)
	return nil
}

type funcAddsuffix struct{ fclosure }

func (f *funcAddsuffix) Arity() int { return 2 }
func (f *funcAddsuffix) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("addsuffix", 2, len(f.args))
	if err != nil {
		return err
	}
	abuf := newEbuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	wb := newWbuf()
	err = f.args[2].Eval(wb, ev)
	if err != nil {
		return err
	}
	t := time.Now()
	suf := abuf.Bytes()
	for _, word := range wb.words {
		var name []byte
		name = append(name, word...)
		name = append(name, suf...)
		w.writeWord(name)
	}
	wb.release()
	abuf.release()
	stats.add("funcbody", "addsuffix", t)
	return err
}

type funcAddprefix struct{ fclosure }

func (f *funcAddprefix) Arity() int { return 2 }
func (f *funcAddprefix) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("addprefix", 2, len(f.args))
	if err != nil {
		return err
	}
	abuf := newEbuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	pre := abuf.Bytes()
	wb := newWbuf()
	err = f.args[2].Eval(wb, ev)
	if err != nil {
		return err
	}
	t := time.Now()
	for _, word := range wb.words {
		var name []byte
		name = append(name, pre...)
		name = append(name, word...)
		w.writeWord(name)
	}
	wb.release()
	abuf.release()
	stats.add("funcbody", "addprefix", t)
	return err
}

type funcRealpath struct{ fclosure }

func (f *funcRealpath) Arity() int { return 1 }
func (f *funcRealpath) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("realpath", 1, len(f.args))
	if err != nil {
		return err
	}
	wb := newWbuf()
	err = f.args[1].Eval(wb, ev)
	if err != nil {
		return err
	}
	if ev.avoidIO {
		fmt.Fprintf(w, "$(realpath %s 2>/dev/null)", string(wb.Bytes()))
		ev.hasIO = true
		wb.release()
		return nil
	}

	t := time.Now()
	for _, word := range wb.words {
		name := string(word)
		name, err := filepath.Abs(name)
		if err != nil {
			glog.Warningf("abs %q: %v", name, err)
			continue
		}
		name, err = filepath.EvalSymlinks(name)
		if err != nil {
			glog.Warningf("realpath %q: %v", name, err)
			continue
		}
		w.writeWordString(name)
	}
	wb.release()
	stats.add("funcbody", "realpath", t)
	return err
}

type funcAbspath struct{ fclosure }

func (f *funcAbspath) Arity() int { return 1 }
func (f *funcAbspath) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("abspath", 1, len(f.args))
	if err != nil {
		return err
	}
	wb := newWbuf()
	err = f.args[1].Eval(wb, ev)
	if err != nil {
		return err
	}
	t := time.Now()
	for _, word := range wb.words {
		name := string(word)
		name, err := filepath.Abs(name)
		if err != nil {
			glog.Warningf("abs %q: %v", name, err)
			continue
		}
		w.writeWordString(name)
	}
	wb.release()
	stats.add("funcbody", "abspath", t)
	return nil
}

// http://www.gnu.org/software/make/manual/make.html#Conditional-Functions
type funcIf struct{ fclosure }

func (f *funcIf) Arity() int { return 3 }
func (f *funcIf) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("if", 2, len(f.args))
	if err != nil {
		return err
	}
	abuf := newEbuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	if len(abuf.Bytes()) != 0 {
		abuf.release()
		return f.args[2].Eval(w, ev)
	}
	abuf.release()
	if len(f.args) > 3 {
		return f.args[3].Eval(w, ev)
	}
	return nil
}

type funcAnd struct{ fclosure }

func (f *funcAnd) Arity() int { return 0 }
func (f *funcAnd) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("and", 0, len(f.args))
	if err != nil {
		return nil
	}
	abuf := newEbuf()
	var cond []byte
	for _, arg := range f.args[1:] {
		abuf.Reset()
		err = arg.Eval(abuf, ev)
		if err != nil {
			return err
		}
		cond = abuf.Bytes()
		if len(cond) == 0 {
			abuf.release()
			return nil
		}
	}
	w.Write(cond)
	abuf.release()
	return nil
}

type funcOr struct{ fclosure }

func (f *funcOr) Arity() int { return 0 }
func (f *funcOr) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("or", 0, len(f.args))
	if err != nil {
		return err
	}
	abuf := newEbuf()
	for _, arg := range f.args[1:] {
		abuf.Reset()
		err = arg.Eval(abuf, ev)
		if err != nil {
			return err
		}
		cond := abuf.Bytes()
		if len(cond) != 0 {
			w.Write(cond)
			abuf.release()
			return nil
		}
	}
	abuf.release()
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
	glog.Infof("has no IO - evaluate now: %s", s)
	return true
}

func (f *funcShell) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("shell", 1, len(f.args))
	if err != nil {
		return err
	}
	abuf := newEbuf()
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
		abuf.release()
		return nil
	}
	arg := abuf.String()
	abuf.release()
	if bc, err := parseBuiltinCommand(arg); err != nil {
		glog.V(1).Infof("sh builtin: %v", err)
	} else {
		glog.Info("use sh builtin:", arg)
		glog.V(2).Infof("builtin command: %#v", bc)
		te := traceEvent.begin("sh-builtin", literal(arg), traceEventMain)
		bc.run(w)
		traceEvent.end(te)
		return nil
	}

	shellVar, err := ev.EvaluateVar("SHELL")
	if err != nil {
		return err
	}
	cmdline := []string{shellVar, "-c", arg}
	if glog.V(1) {
		glog.Infof("shell %q", cmdline)
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
		glog.Warningf("$(shell %q) failed: %q", arg, err)
	}
	w.Write(formatCommandOutput(out))
	traceEvent.end(te)
	return nil
}

func (f *funcShell) Compact() Value {
	if len(f.args)-1 < 1 {
		return f
	}
	if !UseShellBuiltins {
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
				glog.Infof("shell compact apply %s for %s", sb.name, exp)
				return sb.compact(f, v)
			}
		}
		glog.V(1).Infof("shell compact no match: %s", exp)
	}
	return f
}

// https://www.gnu.org/software/make/manual/html_node/Call-Function.html#Call-Function
type funcCall struct{ fclosure }

func (f *funcCall) Arity() int { return 0 }

func (f *funcCall) Eval(w evalWriter, ev *Evaluator) error {
	abuf := newEbuf()
	fargs, err := ev.args(abuf, f.args[1:]...)
	if err != nil {
		return err
	}
	varname := fargs[0]
	variable := string(varname)
	te := traceEvent.begin("call", literal(variable), traceEventMain)
	if glog.V(1) {
		glog.Infof("call %q variable %q", f.args[1], variable)
	}
	v := ev.LookupVar(variable)
	// Evalualte all arguments first before we modify the table.
	// An omitted argument should be blank, even if it's nested inside
	// another call statement that did have that argument passed.
	// see testcases/nested_call.mk
	arglen := len(ev.paramVars)
	if arglen == 0 {
		arglen++
	}
	if arglen < len(fargs[1:])+1 {
		arglen = len(fargs[1:]) + 1
	}
	args := make([]tmpval, arglen)
	// $0 is variable.
	args[0] = tmpval(varname)
	// TODO(ukai): If variable is the name of a built-in function,
	// the built-in function is always invoked (even if a make variable
	// by that name also exists).

	for i, arg := range fargs[1:] {
		// f.args[2]=>args[1] will be $1.
		args[i+1] = tmpval(arg)
		if glog.V(1) {
			glog.Infof("call $%d: %q=>%q", i+1, arg, fargs[i+1])
		}
	}
	oldParams := ev.paramVars
	ev.paramVars = args

	var buf bytes.Buffer
	if glog.V(1) {
		w = &ssvWriter{Writer: io.MultiWriter(w, &buf)}
	}
	err = v.Eval(w, ev)
	if err != nil {
		return err
	}
	ev.paramVars = oldParams
	traceEvent.end(te)
	if glog.V(1) {
		glog.Infof("call %q variable %q return %q", f.args[1], variable, buf.Bytes())
	}
	abuf.release()
	return nil
}

// http://www.gnu.org/software/make/manual/make.html#Value-Function
type funcValue struct{ fclosure }

func (f *funcValue) Arity() int { return 1 }
func (f *funcValue) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("value", 1, len(f.args))
	if err != nil {
		return err
	}
	abuf := newEbuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	v := ev.LookupVar(abuf.String())
	abuf.release()
	io.WriteString(w, v.String())
	return nil
}

// http://www.gnu.org/software/make/manual/make.html#Eval-Function
type funcEval struct{ fclosure }

func (f *funcEval) Arity() int { return 1 }
func (f *funcEval) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("eval", 1, len(f.args))
	if err != nil {
		return err
	}
	abuf := newEbuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	s := abuf.Bytes()
	glog.V(1).Infof("eval %v=>%q at %s", f.args[1], s, ev.srcpos)
	mk, err := parseMakefileBytes(trimSpaceBytes(s), ev.srcpos)
	if err != nil {
		return ev.errorf("%v", err)
	}

	for _, stmt := range mk.stmts {
		err = ev.eval(stmt)
		if err != nil {
			return err
		}
	}
	abuf.release()
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
				glog.V(1).Infof("eval assign %#v => lhs:%q op:%q rhs:%#v", f, lhs, op, rhs)
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

func (f *funcNop) String() string                    { return f.expr }
func (f *funcNop) Eval(evalWriter, *Evaluator) error { return nil }
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

func (f *funcEvalAssign) Eval(w evalWriter, ev *Evaluator) error {
	var abuf evalBuffer
	abuf.resetSep()
	err := f.rhs.Eval(&abuf, ev)
	if err != nil {
		return err
	}
	rhs := trimLeftSpaceBytes(abuf.Bytes())
	glog.V(1).Infof("evalAssign: lhs=%q rhs=%s %q", f.lhs, f.rhs, rhs)
	var rvalue Var
	switch f.op {
	case ":=":
		// TODO(ukai): compute parsed expr in Compact when f.rhs is
		// literal? e.g. literal("$(foo)") => varref{literal("foo")}.
		exp, _, err := parseExpr(rhs, nil, parseOp{})
		if err != nil {
			return ev.errorf("eval assign error: %q: %v", f.String(), err)
		}
		vbuf := newEbuf()
		err = exp.Eval(vbuf, ev)
		if err != nil {
			return err
		}
		rvalue = &simpleVar{value: []string{vbuf.String()}, origin: "file"}
		vbuf.release()
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
	if glog.V(1) {
		glog.Infof("Eval ASSIGN: %s=%q (flavor:%q)", f.lhs, rvalue, rvalue.Flavor())
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
func (f *funcOrigin) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("origin", 1, len(f.args))
	if err != nil {
		return err
	}
	abuf := newEbuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	v := ev.LookupVar(abuf.String())
	abuf.release()
	io.WriteString(w, v.Origin())
	return nil
}

// https://www.gnu.org/software/make/manual/html_node/Flavor-Function.html#Flavor-Function
type funcFlavor struct{ fclosure }

func (f *funcFlavor) Arity() int { return 1 }
func (f *funcFlavor) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("flavor", 1, len(f.args))
	if err != nil {
		return err
	}
	abuf := newEbuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	v := ev.LookupVar(abuf.String())
	abuf.release()
	io.WriteString(w, v.Flavor())
	return nil
}

// http://www.gnu.org/software/make/manual/make.html#Make-Control-Functions
type funcInfo struct{ fclosure }

func (f *funcInfo) Arity() int { return 1 }
func (f *funcInfo) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("info", 1, len(f.args))
	if err != nil {
		return err
	}
	abuf := newEbuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	if ev.avoidIO {
		ev.delayedOutputs = append(ev.delayedOutputs,
			fmt.Sprintf("echo %q", abuf.String()))
		ev.hasIO = true
		abuf.release()
		return nil
	}
	fmt.Printf("%s\n", abuf.String())
	abuf.release()
	return nil
}

type funcWarning struct{ fclosure }

func (f *funcWarning) Arity() int { return 1 }
func (f *funcWarning) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("warning", 1, len(f.args))
	if err != nil {
		return err
	}
	abuf := newEbuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	if ev.avoidIO {
		ev.delayedOutputs = append(ev.delayedOutputs,
			fmt.Sprintf("echo '%s: %s' 1>&2", ev.srcpos, abuf.String()))
		ev.hasIO = true
		abuf.release()
		return nil
	}
	fmt.Printf("%s: %s\n", ev.srcpos, abuf.String())
	abuf.release()
	return nil
}

type funcError struct{ fclosure }

func (f *funcError) Arity() int { return 1 }
func (f *funcError) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("error", 1, len(f.args))
	if err != nil {
		return err
	}
	var abuf evalBuffer
	abuf.resetSep()
	err = f.args[1].Eval(&abuf, ev)
	if err != nil {
		return err
	}
	if ev.avoidIO {
		ev.delayedOutputs = append(ev.delayedOutputs,
			fmt.Sprintf("echo '%s: *** %s.' 1>&2 && false", ev.srcpos, abuf.String()))
		ev.hasIO = true
		abuf.release()
		return nil
	}
	return ev.errorf("*** %s.", abuf.String())
}

// http://www.gnu.org/software/make/manual/make.html#Foreach-Function
type funcForeach struct{ fclosure }

func (f *funcForeach) Arity() int { return 3 }

func (f *funcForeach) Eval(w evalWriter, ev *Evaluator) error {
	err := assertArity("foreach", 3, len(f.args))
	if err != nil {
		return err
	}
	abuf := newEbuf()
	err = f.args[1].Eval(abuf, ev)
	if err != nil {
		return err
	}
	varname := string(abuf.Bytes())
	abuf.release()
	wb := newWbuf()
	err = f.args[2].Eval(wb, ev)
	if err != nil {
		return err
	}
	text := f.args[3]
	ov := ev.LookupVar(varname)
	space := false
	for _, word := range wb.words {
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
	wb.release()
	av := ev.LookupVar(varname)
	if _, ok := av.(*automaticVar); ok {
		ev.outVars.Assign(varname, ov)
	}
	return nil
}
