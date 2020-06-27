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
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/golang/glog"
)

var (
	errEndOfInput = errors.New("unexpected end of input")
	errNotLiteral = errors.New("valueNum: not literal")

	errUnterminatedVariableReference = errors.New("*** unterminated variable reference.")
)

type evalWriter interface {
	io.Writer
	writeWord([]byte)
	writeWordString(string)
	resetSep()
}

// Value is an interface for value.
type Value interface {
	String() string
	Eval(w evalWriter, ev *Evaluator) error
	serialize() serializableVar
	dump(d *dumpbuf)
}

// literal is literal value.
type literal string

func (s literal) String() string { return string(s) }
func (s literal) Eval(w evalWriter, ev *Evaluator) error {
	io.WriteString(w, string(s))
	return nil
}
func (s literal) serialize() serializableVar {
	return serializableVar{Type: "literal", V: string(s)}
}
func (s literal) dump(d *dumpbuf) {
	d.Byte(valueTypeLiteral)
	d.Bytes([]byte(s))
}

// tmpval is temporary value.
type tmpval []byte

func (t tmpval) String() string { return string(t) }
func (t tmpval) Eval(w evalWriter, ev *Evaluator) error {
	w.Write(t)
	return nil
}
func (t tmpval) Value() []byte { return []byte(t) }
func (t tmpval) serialize() serializableVar {
	return serializableVar{Type: "tmpval", V: string(t)}
}
func (t tmpval) dump(d *dumpbuf) {
	d.Byte(valueTypeTmpval)
	d.Bytes(t)
}

// expr is a list of values.
type expr []Value

func (e expr) String() string {
	var s []string
	for _, v := range e {
		s = append(s, v.String())
	}
	return strings.Join(s, "")
}

func (e expr) Eval(w evalWriter, ev *Evaluator) error {
	for _, v := range e {
		w.resetSep()
		err := v.Eval(w, ev)
		if err != nil {
			return err
		}
	}
	return nil
}

func (e expr) serialize() serializableVar {
	r := serializableVar{Type: "expr"}
	for _, v := range e {
		r.Children = append(r.Children, v.serialize())
	}
	return r
}
func (e expr) dump(d *dumpbuf) {
	d.Byte(valueTypeExpr)
	d.Int(len(e))
	for _, v := range e {
		v.dump(d)
	}
}

func compactExpr(e expr) Value {
	if len(e) == 1 {
		return e[0]
	}
	// TODO(ukai): concat literal
	return e
}
func toExpr(v Value) expr {
	if v == nil {
		return nil
	}
	if e, ok := v.(expr); ok {
		return e
	}
	return expr{v}
}

// varref is variable reference. e.g. ${foo}.
type varref struct {
	varname Value
	paren   byte
}

func (v *varref) String() string {
	varname := v.varname.String()
	if len(varname) == 1 && v.paren == 0 {
		return fmt.Sprintf("$%s", varname)
	}
	paren := v.paren
	if paren == 0 {
		paren = '{'
	}
	return fmt.Sprintf("$%c%s%c", paren, varname, closeParen(paren))
}

func (v *varref) Eval(w evalWriter, ev *Evaluator) error {
	te := traceEvent.begin("var", v, traceEventMain)
	buf := newEbuf()
	err := v.varname.Eval(buf, ev)
	if err != nil {
		return err
	}
	vv := ev.LookupVar(buf.String())
	buf.release()
	err = vv.Eval(w, ev)
	if err != nil {
		return err
	}
	traceEvent.end(te)
	return nil
}

func (v *varref) serialize() serializableVar {
	return serializableVar{
		Type:     "varref",
		V:        string(v.paren),
		Children: []serializableVar{v.varname.serialize()},
	}
}
func (v *varref) dump(d *dumpbuf) {
	d.Byte(valueTypeVarref)
	d.Byte(v.paren)
	v.varname.dump(d)
}

// paramref is parameter reference e.g. $1.
type paramref int

func (p paramref) String() string {
	return fmt.Sprintf("$%d", int(p))
}

func (p paramref) Eval(w evalWriter, ev *Evaluator) error {
	te := traceEvent.begin("param", p, traceEventMain)
	n := int(p)
	if n < len(ev.paramVars) {
		err := ev.paramVars[n].Eval(w, ev)
		if err != nil {
			return err
		}
	} else {
		vv := ev.LookupVar(fmt.Sprintf("%d", n))
		err := vv.Eval(w, ev)
		if err != nil {
			return err
		}
	}
	traceEvent.end(te)
	return nil
}

func (p paramref) serialize() serializableVar {
	return serializableVar{Type: "paramref", V: strconv.Itoa(int(p))}
}

func (p paramref) dump(d *dumpbuf) {
	d.Byte(valueTypeParamref)
	d.Int(int(p))
}

// varsubst is variable substitutaion. e.g. ${var:pat=subst}.
type varsubst struct {
	varname Value
	pat     Value
	subst   Value
	paren   byte
}

func (v varsubst) String() string {
	paren := v.paren
	if paren == 0 {
		paren = '{'
	}
	return fmt.Sprintf("$%c%s:%s=%s%c", paren, v.varname, v.pat, v.subst, closeParen(paren))
}

func (v varsubst) Eval(w evalWriter, ev *Evaluator) error {
	te := traceEvent.begin("varsubst", v, traceEventMain)
	buf := newEbuf()
	params, err := ev.args(buf, v.varname, v.pat, v.subst)
	if err != nil {
		return err
	}
	vname := string(params[0])
	pat := string(params[1])
	subst := string(params[2])
	buf.Reset()
	vv := ev.LookupVar(vname)
	err = vv.Eval(buf, ev)
	if err != nil {
		return err
	}
	vals := splitSpaces(buf.String())
	buf.release()
	space := false
	for _, val := range vals {
		if space {
			io.WriteString(w, " ")
		}
		io.WriteString(w, substRef(pat, subst, val))
		space = true
	}
	traceEvent.end(te)
	return nil
}

func (v varsubst) serialize() serializableVar {
	return serializableVar{
		Type: "varsubst",
		V:    string(v.paren),
		Children: []serializableVar{
			v.varname.serialize(),
			v.pat.serialize(),
			v.subst.serialize(),
		},
	}
}

func (v varsubst) dump(d *dumpbuf) {
	d.Byte(valueTypeVarsubst)
	d.Byte(v.paren)
	v.varname.dump(d)
	v.pat.dump(d)
	v.subst.dump(d)
}

func str(buf []byte, alloc bool) Value {
	if alloc {
		return literal(string(buf))
	}
	return tmpval(buf)
}

func appendStr(exp expr, buf []byte, alloc bool) expr {
	if len(buf) == 0 {
		return exp
	}
	if len(exp) == 0 {
		return append(exp, str(buf, alloc))
	}
	switch v := exp[len(exp)-1].(type) {
	case literal:
		v += literal(string(buf))
		exp[len(exp)-1] = v
		return exp
	case tmpval:
		v = append(v, buf...)
		exp[len(exp)-1] = v
		return exp
	}
	return append(exp, str(buf, alloc))
}

func valueNum(v Value) (int, error) {
	switch v := v.(type) {
	case literal, tmpval:
		n, err := strconv.ParseInt(v.String(), 10, 64)
		return int(n), err
	}
	return 0, errNotLiteral
}

type parseOp struct {
	// alloc indicates text will be allocated as literal (string)
	alloc bool

	// matchParen matches parenthesis.
	// note: required for func arg
	matchParen bool
}

// parseExpr parses expression in `in` until it finds any byte in term.
// if term is nil, it will parse to end of input.
// if term is not nil, and it reaches to end of input, return error.
// it returns parsed value, and parsed length `n`, so in[n-1] is any byte of
// term, and in[n:] is next input.
func parseExpr(in, term []byte, op parseOp) (Value, int, error) {
	var exp expr
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
				exp = appendStr(exp, in[b:i+1], op.alloc)
				i += 2
				b = i
				continue
			}
			if bytes.IndexByte(term, in[i+1]) >= 0 {
				exp = appendStr(exp, in[b:i], op.alloc)
				exp = append(exp, &varref{varname: literal("")})
				i++
				b = i
				break Loop
			}
			exp = appendStr(exp, in[b:i], op.alloc)
			v, n, err := parseDollar(in[i:], op.alloc)
			if err != nil {
				return nil, 0, err
			}
			i += n
			b = i
			exp = append(exp, v)
			continue
		case '(', '{':
			if !op.matchParen {
				break
			}
			cp := closeParen(ch)
			if i := bytes.IndexByte(term, cp); i >= 0 {
				parenDepth++
				saveParen = cp
				term[i] = 0
			} else if cp == saveParen {
				parenDepth++
			}
		case saveParen:
			if !op.matchParen {
				break
			}
			parenDepth--
			if parenDepth == 0 {
				i := bytes.IndexByte(term, 0)
				term[i] = saveParen
				saveParen = 0
			}
		}
		i++
	}
	exp = appendStr(exp, in[b:i], op.alloc)
	if i == len(in) && term != nil {
		glog.Warningf("parse: unexpected end of input: %q %d [%q]", in, i, term)
		return exp, i, errEndOfInput
	}
	return compactExpr(exp), i, nil
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
	oparen := in[1]
	paren := closeParen(oparen)
	if paren == 0 {
		// $x case.
		if in[1] >= '0' && in[1] <= '9' {
			return paramref(in[1] - '0'), 2, nil
		}
		return &varref{varname: str(in[1:2], alloc)}, 2, nil
	}
	term := []byte{paren, ':', ' '}
	var varname expr
	i := 2
	op := parseOp{alloc: alloc}
Again:
	for {
		e, n, err := parseExpr(in[i:], term, op)
		if err != nil {
			if err == errEndOfInput {
				// unmatched_paren2.mk
				varname = append(varname, toExpr(e)...)
				if len(varname) > 0 {
					for i, vn := range varname {
						if vr, ok := vn.(*varref); ok {
							if vr.paren == oparen {
								varname = varname[:i+1]
								varname[i] = expr{literal(fmt.Sprintf("$%c", oparen)), vr.varname}
								return &varref{varname: varname, paren: oparen}, i + 1 + n + 1, nil
							}
						}
					}
				}
				return nil, 0, errUnterminatedVariableReference
			}
			return nil, 0, err
		}
		varname = append(varname, toExpr(e)...)
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
			return &varref{varname: vname, paren: oparen}, i + 1, nil
		case ' ':
			// ${e ...}
			switch token := e.(type) {
			case literal, tmpval:
				funcName := intern(token.String())
				if f, ok := funcMap[funcName]; ok {
					return parseFunc(f(), in, i+1, term[:1], funcName, op.alloc)
				}
			}
			term = term[:2] // drop ' '
			continue Again
		case ':':
			// ${varname:...}
			colon := in[i : i+1]
			var vterm []byte
			vterm = append(vterm, term[:2]...)
			vterm[1] = '=' // term={paren, '='}.
			e, n, err := parseExpr(in[i+1:], vterm, op)
			if err != nil {
				return nil, 0, err
			}
			i += 1 + n
			if in[i] == paren {
				varname = appendStr(varname, colon, op.alloc)
				return &varref{varname: varname, paren: oparen}, i + 1, nil
			}
			// ${varname:xx=...}
			pat := e
			subst, n, err := parseExpr(in[i+1:], term[:1], op)
			if err != nil {
				return nil, 0, err
			}
			i += 1 + n
			// ${first:pat=e}
			return varsubst{
				varname: compactExpr(varname),
				pat:     pat,
				subst:   subst,
				paren:   oparen,
			}, i + 1, nil
		default:
			return nil, 0, fmt.Errorf("unexpected char %c at %d in %q", in[i], i, string(in))
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
	case expr:
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
	case expr:
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
func parseFunc(f mkFunc, in []byte, s int, term []byte, funcName string, alloc bool) (Value, int, error) {
	f.AddArg(str(in[1:s-1], alloc))
	arity := f.Arity()
	term = append(term, ',')
	i := skipSpaces(in[s:], term)
	i = s + i
	if i == len(in) {
		return f, i, nil
	}
	narg := 1
	op := parseOp{alloc: alloc, matchParen: true}
	for {
		if arity != 0 && narg >= arity {
			// final arguments.
			term = term[:1] // drop ','
		}
		v, n, err := parseExpr(in[i:], term, op)
		if err != nil {
			if err == errEndOfInput {
				return nil, 0, fmt.Errorf("*** unterminated call to function `%s': missing `)'.", funcName)
			}
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
	if compactor, ok := f.(compactor); ok {
		fv = compactor.Compact()
	}
	if EvalStatsFlag || traceEvent.enabled() {
		fv = funcstats{
			Value: fv,
			str:   fv.String(),
		}

	}
	return fv, i, nil
}

type compactor interface {
	Compact() Value
}

type funcstats struct {
	Value
	str string
}

func (f funcstats) Eval(w evalWriter, ev *Evaluator) error {
	te := traceEvent.begin("func", literal(f.str), traceEventMain)
	err := f.Value.Eval(w, ev)
	if err != nil {
		return err
	}
	// TODO(ukai): per functype?
	traceEvent.end(te)
	return nil
}

type matcherValue struct{}

func (m matcherValue) Eval(w evalWriter, ev *Evaluator) error {
	return fmt.Errorf("couldn't eval matcher")
}
func (m matcherValue) serialize() serializableVar {
	return serializableVar{Type: ""}
}

func (m matcherValue) dump(d *dumpbuf) {
	d.err = fmt.Errorf("couldn't dump matcher")
}

type matchVarref struct{ matcherValue }

func (m matchVarref) String() string { return "$(match-any)" }

type literalRE struct {
	matcherValue
	*regexp.Regexp
}

func mustLiteralRE(s string) literalRE {
	return literalRE{
		Regexp: regexp.MustCompile(s),
	}
}

func (r literalRE) String() string { return r.Regexp.String() }

func matchValue(exp, pat Value) bool {
	switch pat := pat.(type) {
	case literal:
		return literal(exp.String()) == pat
	}
	// TODO: other type match?
	return false
}

func matchExpr(exp, pat expr) ([]Value, bool) {
	if len(exp) != len(pat) {
		return nil, false
	}
	var mv matchVarref
	var matches []Value
	for i := range exp {
		if pat[i] == mv {
			switch exp[i].(type) {
			case paramref, *varref:
				matches = append(matches, exp[i])
				continue
			}
			return nil, false
		}
		if patre, ok := pat[i].(literalRE); ok {
			re := patre.Regexp
			m := re.FindStringSubmatch(exp[i].String())
			if m == nil {
				return nil, false
			}
			for _, sm := range m[1:] {
				matches = append(matches, literal(sm))
			}
			continue
		}
		if !matchValue(exp[i], pat[i]) {
			return nil, false
		}
	}
	return matches, true
}
