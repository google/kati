package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

var (
	errEndOfInput = errors.New("parse: unexpected end of input")
)

type Value interface {
	String() string
	Eval(w io.Writer, ev *Evaluator)
}

type Valuer interface {
	Value() []byte
}

// literal is literal value.
// TODO(ukai): always use []byte?
type literal string

func (s literal) String() string { return string(s) }
func (s literal) Eval(w io.Writer, ev *Evaluator) {
	fmt.Fprint(w, string(s))
}

// tmpval is temporary value.
// TODO(ukai): Values() returns []Value? (word list?)
type tmpval []byte

func (t tmpval) String() string { return string(t) }
func (t tmpval) Eval(w io.Writer, ev *Evaluator) {
	w.Write(t)
}
func (t tmpval) Value() []byte { return []byte(t) }

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
	t := time.Now()
	vname := ev.Value(v.varname)
	vv := ev.LookupVar(string(vname))
	vv.Eval(w, ev)
	addStats("var", v, t)
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
	t := time.Now()
	vname := ev.Value(v.varname)
	vv := ev.LookupVar(string(vname))
	vals := ev.Values(vv)
	pat := ev.Value(v.pat)
	subst := ev.Value(v.subst)
	space := false
	for _, val := range vals {
		if space {
			fmt.Fprint(w, " ")
		}
		fmt.Fprint(w, substRef(string(pat), string(subst), string(val)))
		space = true
	}
	addStats("varsubst", v, t)
}

// parseExpr parses expression in `in` until it finds any byte in term.
// if term is nil, it will parse to end of input.
// if term is not nil, and it reaches to end of input, return errEndOfInput.
// it returns parsed value, and parsed length `n`, so in[n-1] is any byte of
// term, and in[n:] is next input.
func parseExpr(in, term []byte) (Value, int, error) {
	return parseExprImpl(in, term, false, false)
}

func parseExprImpl(in, term []byte, trimSpace bool, inFunc bool) (Value, int, error) {
	var expr Expr
	var buf bytes.Buffer
	i := 0
	var saveParen byte
	parenDepth := 0
	if trimSpace {
		for i < len(in) && (in[i] == ' ' || in[i] == '\t') {
			i++
		}
	}
Loop:
	for i < len(in) {
		ch := in[i]
		if bytes.IndexByte(term, ch) >= 0 {
			break Loop
		}
		switch ch {
		case '$':
			if i+1 >= len(in) {
				break Loop
			}
			if in[i+1] == '$' {
				buf.WriteByte('$')
				i += 2
				continue
			}
			if bytes.IndexByte(term, in[i+1]) >= 0 {
				expr = append(expr, varref{varname: literal("")})
				i++
				break Loop
			}
			if buf.Len() > 0 {
				expr = append(expr, literal(buf.String()))
				buf.Reset()
			}
			v, n, err := parseDollar(in[i:])
			if err != nil {
				return nil, 0, err
			}
			i += n
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
		case '\\':
			// If you find '\' followed by '\n' in a
			// function call, we need to handle it as if
			// we are not in a recipe. See also
			// processMakefileLine.
			if inFunc && i+1 < len(in) && in[i+1] == '\n' {
				trimmed := bytes.TrimRight(buf.Bytes(), "\t ")
				buf.Reset()
				buf.Write(trimmed)
				buf.WriteByte(' ')
				i = i + 2
				for i < len(in) && (in[i] == ' ' || in[i] == '\t') {
					Log("fuck")
					i++
				}
				continue
			}
		}
		buf.WriteByte(ch)
		i++
	}
	if buf.Len() > 0 {
		s := buf.String()
		if trimSpace {
			s = strings.TrimSpace(s)
		}
		expr = append(expr, literal(s))
	}
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
func parseDollar(in []byte) (Value, int, error) {
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
		return varref{varname: literal(string(in[1]))}, 2, nil
	}
	term := []byte{paren, ':', ' '}
	var varname Expr
	i := 2
Again:
	for {
		e, n, err := parseExpr(in[i:], term)
		if err != nil {
			return nil, 0, err
		}
		varname = append(varname, e)
		i += n
		switch in[i] {
		case paren:
			// ${expr}
			return varref{varname: compactExpr(varname)}, i + 1, nil
		case ' ':
			// ${e ...}
			if token, ok := e.(literal); ok {
				funcName := string(token)
				if f, ok := funcMap[funcName]; ok {
					return parseFunc(f(), in, i+1, term[:1], funcName)
				}
			}
			term = term[:2] // drop ' '
			continue Again
		case ':':
			// ${varname:...}
			term = term[:2]
			term[1] = '=' // term={paren, '='}.
			e, n, err := parseExpr(in[i+1:], term)
			if err != nil {
				return nil, 0, err
			}
			i += 1 + n
			if in[i] == paren {
				varname = append(varname, literal(string(":")), e)
				return varref{varname: varname}, i + 1, nil
			}
			// ${varname:xx=...}
			pat := e
			subst, n, err := parseExpr(in[i+1:], term[:1])
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

// parseFunc parses function arguments from in[s:] for f.
// in[:n] will be "${func args...}"
func parseFunc(f Func, in []byte, s int, term []byte, funcName string) (Value, int, error) {
	arity := f.Arity()
	term = append(term, ',')
	i := skipSpaces(in[s:], term)
	i = s + i
	if i == len(in) {
		f.SetString(string(in[:i]))
		return f, i, nil
	}
	narg := 1
	for {
		if arity != 0 && narg >= arity {
			// final arguments.
			term = term[:1] // drop ','
		}
		trimSpace := (narg == 1 && funcName == "if") || funcName == "and" || funcName == "or"
		v, n, err := parseExprImpl(in[i:], term, trimSpace, true)
		if err != nil {
			return nil, 0, err
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
	f.SetString(string(in[:i]))
	if katiStatsFlag {
		f = funcstats{f}
	}
	return f, i, nil
}

type funcstats struct {
	Func
}

func (f funcstats) Eval(w io.Writer, ev *Evaluator) {
	t := time.Now()
	f.Func.Eval(w, ev)
	// TODO(ukai): per functype?
	addStats("func", f, t)
}
