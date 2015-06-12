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

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

var (
	errEndOfInput = errors.New("parse: unexpected end of input")
	errNotLiteral  = errors.New("valueNum: not literal")

	bufFree = sync.Pool{
		New: func() interface{} { return new(buffer) },
	}
)

type buffer struct {
	bytes.Buffer
	args [][]byte
}

func newBuf() *buffer {
	buf := bufFree.Get().(*buffer)
	return buf
}

func freeBuf(buf *buffer) {
	if cap(buf.Bytes()) > 1024 {
		return
	}
	buf.Reset()
	buf.args = buf.args[:0]
	bufFree.Put(buf)
}

type Value interface {
	String() string
	Eval(w io.Writer, ev *Evaluator)
	Serialize() SerializableVar
	Dump(w io.Writer)
}

type Valuer interface {
	Value() []byte
}

// literal is literal value.
type literal string

func (s literal) String() string { return string(s) }
func (s literal) Eval(w io.Writer, ev *Evaluator) {
	io.WriteString(w, string(s))
}
func (s literal) Serialize() SerializableVar {
	return SerializableVar{Type: "literal", V: string(s)}
}
func (s literal) Dump(w io.Writer) {
	dumpByte(w, ValueTypeLiteral)
	dumpBytes(w, []byte(s))
}

// tmpval is temporary value.
type tmpval []byte

func (t tmpval) String() string { return string(t) }
func (t tmpval) Eval(w io.Writer, ev *Evaluator) {
	w.Write(t)
}
func (t tmpval) Value() []byte { return []byte(t) }
func (t tmpval) Serialize() SerializableVar {
	return SerializableVar{Type: "tmpval", V: string(t)}
}
func (t tmpval) Dump(w io.Writer) {
	dumpByte(w, ValueTypeTmpval)
	dumpBytes(w, t)
}

// Expr is a list of values.
type Expr []Value

func (e Expr) String() string {
	var s []string
	for _, v := range e {
		s = append(s, v.String())
	}
	return strings.Join(s, "")
}

func (e Expr) Eval(w io.Writer, ev *Evaluator) {
	for _, v := range e {
		v.Eval(w, ev)
	}
}

func (e Expr) Serialize() SerializableVar {
	r := SerializableVar{Type: "expr"}
	for _, v := range e {
		r.Children = append(r.Children, v.Serialize())
	}
	return r
}
func (e Expr) Dump(w io.Writer) {
	dumpByte(w, ValueTypeExpr)
	dumpInt(w, len(e))
	for _, v := range e {
		v.Dump(w)
	}
}

func compactExpr(e Expr) Value {
	if len(e) == 1 {
		return e[0]
	}
	// TODO(ukai): concat literal
	return e
}

// varref is variable reference. e.g. ${foo}.
type varref struct {
	varname Value
}

func (v varref) String() string {
	varname := v.varname.String()
	if len(varname) == 1 {
		return fmt.Sprintf("$%s", varname)
	}
	return fmt.Sprintf("${%s}", varname)
}

func (v varref) Eval(w io.Writer, ev *Evaluator) {
	te := traceEvent.begin("var", v, traceEventMain)
	buf := newBuf()
	v.varname.Eval(buf, ev)
	vv := ev.LookupVar(buf.String())
	freeBuf(buf)
	vv.Eval(w, ev)
	traceEvent.end(te)
}

func (v varref) Serialize() SerializableVar {
	return SerializableVar{
		Type:     "varref",
		Children: []SerializableVar{v.varname.Serialize()},
	}
}
func (v varref) Dump(w io.Writer) {
	dumpByte(w, ValueTypeVarref)
	v.varname.Dump(w)
}

// paramref is parameter reference e.g. $1.
type paramref int

func (p paramref) String() string {
	return fmt.Sprintf("$%d", int(p))
}

func (p paramref) Eval(w io.Writer, ev *Evaluator) {
	te := traceEvent.begin("param", p, traceEventMain)
	n := int(p)
	if n < len(ev.paramVars) {
		ev.paramVars[n].Eval(w, ev)
	} else {
		// out of range?
		// panic(fmt.Sprintf("out of range %d: %d", n, len(ev.paramVars)))
	}
	traceEvent.end(te)
}

func (p paramref) Serialize() SerializableVar {
	return SerializableVar{Type: "paramref", V: strconv.Itoa(int(p))}
}

func (p paramref) Dump(w io.Writer) {
	dumpByte(w, ValueTypeParamref)
	dumpInt(w, int(p))
}

// varsubst is variable substitutaion. e.g. ${var:pat=subst}.
type varsubst struct {
	varname Value
	pat     Value
	subst   Value
}

func (v varsubst) String() string {
	return fmt.Sprintf("${%s:%s=%s}", v.varname, v.pat, v.subst)
}

func (v varsubst) Eval(w io.Writer, ev *Evaluator) {
	te := traceEvent.begin("varsubst", v, traceEventMain)
	buf := newBuf()
	params := ev.args(buf, v.varname, v.pat, v.subst)
	vname := string(params[0])
	pat := string(params[1])
	subst := string(params[2])
	buf.Reset()
	vv := ev.LookupVar(vname)
	vv.Eval(buf, ev)
	vals := splitSpaces(buf.String())
	freeBuf(buf)
	space := false
	for _, val := range vals {
		if space {
			io.WriteString(w, " ")
		}
		io.WriteString(w, substRef(pat, subst, val))
		space = true
	}
	traceEvent.end(te)
}

func (v varsubst) Serialize() SerializableVar {
	return SerializableVar{
		Type: "varsubst",
		Children: []SerializableVar{
			v.varname.Serialize(),
			v.pat.Serialize(),
			v.subst.Serialize(),
		},
	}
}

func (v varsubst) Dump(w io.Writer) {
	dumpByte(w, ValueTypeVarsubst)
	v.varname.Dump(w)
	v.pat.Dump(w)
	v.subst.Dump(w)
}

func str(buf []byte, alloc bool) Value {
	if alloc {
		return literal(string(buf))
	}
	return tmpval(buf)
}

func appendStr(expr Expr, buf []byte, alloc bool) Expr {
	if len(buf) == 0 {
		return expr
	}
	if len(expr) == 0 {
		return Expr{str(buf, alloc)}
	}
	switch v := expr[len(expr)-1].(type) {
	case literal:
		v += literal(string(buf))
		expr[len(expr)-1] = v
		return expr
	case tmpval:
		v = append(v, buf...)
		expr[len(expr)-1] = v
		return expr
	}
	return append(expr, str(buf, alloc))
}

func valueNum(v Value) (int, error) {
	switch v := v.(type) {
	case literal, tmpval:
		n, err := strconv.ParseInt(v.String(), 10, 64)
		return int(n), err
	}
	return 0, errNotLiteral
}

// parseExpr parses expression in `in` until it finds any byte in term.
// if term is nil, it will parse to end of input.
// if term is not nil, and it reaches to end of input, return errEndOfInput.
// it returns parsed value, and parsed length `n`, so in[n-1] is any byte of
// term, and in[n:] is next input.
// if alloc is true, text will be literal (allocate string).
// otherwise, text will be tmpval on in.
func parseExpr(in, term []byte, alloc bool) (Value, int, error) {
	var expr Expr
	b := 0
	i := 0
	var saveParen byte
	parenDepth := 0
Loop:
	for i < len(in) {
		ch := in[i]
		if term != nil && bytes.IndexByte(term, ch) >= 0 {
			break Loop
		}
		switch ch {
		case '$':
			if i+1 >= len(in) {
				break Loop
			}
			if in[i+1] == '$' {
				expr = appendStr(expr, in[b:i+1], alloc)
				i += 2
				b = i
				continue
			}
			if bytes.IndexByte(term, in[i+1]) >= 0 {
				expr = appendStr(expr, in[b:i], alloc)
				expr = append(expr, varref{varname: literal("")})
				i++
				b = i
				break Loop
			}
			expr = appendStr(expr, in[b:i], alloc)
			v, n, err := parseDollar(in[i:], alloc)
			if err != nil {
				return nil, 0, err
			}
			i += n
			b = i
			expr = append(expr, v)
			continue
		case '(', '{':
			cp := closeParen(ch)
			if i := bytes.IndexByte(term, cp); i >= 0 {
				parenDepth++
				saveParen = cp
				term[i] = 0
			} else if cp == saveParen {
				parenDepth++
			}
		case saveParen:
			parenDepth--
			if parenDepth == 0 {
				i := bytes.IndexByte(term, 0)
				term[i] = saveParen
				saveParen = 0
			}
		}
		i++
	}
	expr = appendStr(expr, in[b:i], alloc)
	if i == len(in) && term != nil {
		return expr, i, errEndOfInput
	}
	return compactExpr(expr), i, nil
}

func closeParen(ch byte) byte {
	switch ch {
	case '(':
		return ')'
	case '{':
		return '}'
	}
	return 0
}

// parseDollar parses
//   $(func expr[, expr...])  # func = literal SP
//   $(expr:expr=expr)
//   $(expr)
//   $x
// it returns parsed value and parsed length.
func parseDollar(in []byte, alloc bool) (Value, int, error) {
	if len(in) <= 1 {
		return nil, 0, errors.New("empty expr")
	}
	if in[0] != '$' {
		return nil, 0, errors.New("should starts with $")
	}
	if in[1] == '$' {
		return nil, 0, errors.New("should handle $$ as literal $")
	}
	paren := closeParen(in[1])
	if paren == 0 {
		// $x case.
		if in[1] >= '0' && in[1] <= '9' {
			return paramref(in[1] - '0'), 2, nil
		}
		return varref{varname: str(in[1:2], alloc)}, 2, nil
	}
	term := []byte{paren, ':', ' '}
	var varname Expr
	i := 2
Again:
	for {
		e, n, err := parseExpr(in[i:], term, alloc)
		if err != nil {
			return nil, 0, err
		}
		varname = append(varname, e)
		i += n
		switch in[i] {
		case paren:
			// ${expr}
			vname := compactExpr(varname)
			n, err := valueNum(vname)
			if err == nil {
				// ${n}
				return paramref(n), i + 1, nil
			}
			return varref{varname: vname}, i + 1, nil
		case ' ':
			// ${e ...}
			switch token := e.(type) {
			case literal, tmpval:
				funcName := intern(token.String())
				if f, ok := funcMap[funcName]; ok {
					return parseFunc(f(), in, i+1, term[:1], funcName, alloc)
				}
			}
			term = term[:2] // drop ' '
			continue Again
		case ':':
			// ${varname:...}
			colon := in[i:i+1]
			term = term[:2]
			term[1] = '=' // term={paren, '='}.
			e, n, err := parseExpr(in[i+1:], term, alloc)
			if err != nil {
				return nil, 0, err
			}
			i += 1 + n
			if in[i] == paren {
				varname = appendStr(varname, colon, alloc)
				return varref{varname: varname}, i + 1, nil
			}
			// ${varname:xx=...}
			pat := e
			subst, n, err := parseExpr(in[i+1:], term[:1], alloc)
			if err != nil {
				return nil, 0, err
			}
			i += 1 + n
			// ${first:pat=e}
			return varsubst{
				varname: compactExpr(varname),
				pat:     pat,
				subst:   subst,
			}, i + 1, nil
		default:
			panic(fmt.Sprintf("unexpected char"))
		}
	}
}

// skipSpaces skips spaces at front of `in` before any bytes in term.
// in[n] will be the first non white space in in.
func skipSpaces(in, term []byte) int {
	for i := 0; i < len(in); i++ {
		if bytes.IndexByte(term, in[i]) >= 0 {
			return i
		}
		switch in[i] {
		case ' ', '\t':
		default:
			return i
		}
	}
	return len(in)
}

// trimLiteralSpace trims literal space around v.
func trimLiteralSpace(v Value) Value {
	switch v := v.(type) {
	case literal:
		return literal(strings.TrimSpace(string(v)))
	case tmpval:
		b := bytes.TrimSpace([]byte(v))
		if len(b) == 0 {
			return literal("")
		}
		return tmpval(b)
	case Expr:
		if len(v) == 0 {
			return v
		}
		switch s := v[0].(type) {
		case literal, tmpval:
			t := trimLiteralSpace(s)
			if t == literal("") {
				v = v[1:]
			} else {
				v[0] = t
			}
		}
		switch s := v[len(v)-1].(type) {
		case literal, tmpval:
			t := trimLiteralSpace(s)
			if t == literal("") {
				v = v[:len(v)-1]
			} else {
				v[len(v)-1] = t
			}
		}
		return compactExpr(v)
	}
	return v
}

// concatLine concatinates line with "\\\n" in function expression.
// TODO(ukai): less alloc?
func concatLine(v Value) Value {
	switch v := v.(type) {
	case literal:
		for {
			s := string(v)
			i := strings.Index(s, "\\\n")
			if i < 0 {
				return v
			}
			v = literal(s[:i] + strings.TrimLeft(s[i+2:], " \t"))
		}
	case tmpval:
		for {
			b := []byte(v)
			i := bytes.Index(b, []byte{'\\', '\n'})
			if i < 0 {
				return v
			}
			var buf bytes.Buffer
			buf.Write(b[:i])
			buf.Write(bytes.TrimLeft(b[i+2:], " \t"))
			v = tmpval(buf.Bytes())
		}
	case Expr:
		for i := range v {
			switch vv := v[i].(type) {
			case literal, tmpval:
				v[i] = concatLine(vv)
			}
		}
		return v
	}
	return v
}

// parseFunc parses function arguments from in[s:] for f.
// in[0] is '$' and in[s] is space just after func name.
// in[:n] will be "${func args...}"
func parseFunc(f Func, in []byte, s int, term []byte, funcName string, alloc bool) (Value, int, error) {
	f.AddArg(str(in[1:s-1], alloc))
	arity := f.Arity()
	term = append(term, ',')
	i := skipSpaces(in[s:], term)
	i = s + i
	if i == len(in) {
		return f, i, nil
	}
	narg := 1
	for {
		if arity != 0 && narg >= arity {
			// final arguments.
			term = term[:1] // drop ','
		}
		v, n, err := parseExpr(in[i:], term, alloc)
		if err != nil {
			return nil, 0, err
		}
		v = concatLine(v)
		// TODO(ukai): do this in funcIf, funcAnd, or funcOr's compactor?
		if (narg == 1 && funcName == "if") || funcName == "and" || funcName == "or" {
			v = trimLiteralSpace(v)
		}
		f.AddArg(v)
		i += n
		narg++
		if in[i] == term[0] {
			i++
			break
		}
		i++ // should be ','
		if i == len(in) {
			break
		}
	}
	var fv Value
	fv = f
	if compactor, ok := f.(Compactor); ok {
		fv = compactor.Compact()
	}
	if katiEvalStatsFlag || traceEvent.enabled() {
		fv = funcstats{
			Value: fv,
			str:   fv.String(),
		}

	}
	return fv, i, nil
}

type Compactor interface {
	Compact() Value
}

type funcstats struct {
	Value
	str string
}

func (f funcstats) Eval(w io.Writer, ev *Evaluator) {
	te := traceEvent.begin("func", literal(f.str), traceEventMain)
	f.Value.Eval(w, ev)
	// TODO(ukai): per functype?
	traceEvent.end(te)
}

type matchVarref struct{}

func (m matchVarref) String() string                  { return "$(match-any)" }
func (m matchVarref) Eval(w io.Writer, ev *Evaluator) { panic("not implemented") }
func (m matchVarref) Serialize() SerializableVar      { panic("not implemented") }
func (m matchVarref) Dump(w io.Writer)                { panic("not implemented") }

func matchValue(expr, pat Value) bool {
	switch pat := pat.(type) {
	case literal:
		return literal(expr.String()) == pat
	}
	// TODO: other type match?
	return false
}

func matchExpr(expr, pat Expr) ([]Value, bool) {
	if len(expr) != len(pat) {
		return nil, false
	}
	var mv matchVarref
	var matches []Value
	for i := range expr {
		if pat[i] == mv {
			switch expr[i].(type) {
			case paramref, varref:
				matches = append(matches, expr[i])
				continue
			}
			return nil, false
		}
		if !matchValue(expr[i], pat[i]) {
			return nil, false
		}
	}
	return matches, true
}
