package main

import (
	"bytes"
	"errors"
	"strings"
)

type pattern struct {
	prefix, suffix string
}

func (p pattern) String() string {
	return p.prefix + "%" + p.suffix
}

func (p pattern) match(s string) bool {
	return strings.HasPrefix(s, p.prefix) && strings.HasSuffix(s, p.suffix)
}

func (p pattern) subst(repl, str string) string {
	in := str
	trimed := str
	if p.prefix != "" {
		trimed = strings.TrimPrefix(in, p.prefix)
		if trimed == in {
			return str
		}
	}
	in = trimed
	if p.suffix != "" {
		trimed = strings.TrimSuffix(in, p.suffix)
		if trimed == in {
			return str
		}
	}
	rs := strings.SplitN(repl, "%", 2)
	if len(rs) != 2 {
		return repl
	}
	return rs[0] + trimed + rs[1]
}

type Rule struct {
	outputs         []string
	inputs          []string
	orderOnlyInputs []string
	outputPatterns  []pattern
	isDoubleColon   bool
	isSuffixRule    bool
	cmds            []string
	filename        string
	lineno          int
	cmdLineno       int
}

func isPatternRule(s []byte) (pattern, bool) {
	i := bytes.IndexByte(s, '%')
	if i < 0 {
		return pattern{}, false
	}
	return pattern{prefix: string(s[:i]), suffix: string(s[i+1:])}, true
}

func (r *Rule) parseInputs(s []byte) {
	ws := newWordScanner(s)
	isOrderOnly := false
	for ws.Scan() {
		input := ws.Bytes()
		if len(input) == 1 && input[0] == '|' {
			isOrderOnly = true
			continue
		}
		if isOrderOnly {
			r.orderOnlyInputs = append(r.orderOnlyInputs, internBytes(input))
		} else {
			r.inputs = append(r.inputs, internBytes(input))
		}
	}
}

func (r *Rule) parseVar(s []byte) *AssignAST {
	eq := bytes.IndexByte(s, '=')
	if eq <= 0 {
		return nil
	}
	rhs := trimLeftSpaceBytes(s[eq+1:])
	var lhs []byte
	var op string
	// TODO(ukai): support override, export.
	switch s[eq-1] { // s[eq] is '='
	case ':':
		lhs = trimSpaceBytes(s[:eq-1])
		op = ":="
	case '+':
		lhs = trimSpaceBytes(s[:eq-1])
		op = "+="
	case '?':
		lhs = trimSpaceBytes(s[:eq-1])
		op = "?="
	default:
		lhs = trimSpaceBytes(s[:eq])
		op = "="
	}
	assign := newAssignAST(nil, lhs, rhs, op)
	assign.filename = r.filename
	assign.lineno = r.lineno
	return assign
}

func (r *Rule) parse(line []byte) (*AssignAST, error) {
	index := bytes.IndexByte(line, ':')
	if index < 0 {
		return nil, errors.New("*** missing separator.")
	}

	first := line[:index]
	ws := newWordScanner(first)
	pat, isFirstPattern := isPatternRule(first)
	if isFirstPattern {
		n := 0
		for ws.Scan() {
			n++
			if n > 1 {
				return nil, errors.New("*** mixed implicit and normal rules: deprecated syntax")
			}
		}
		r.outputPatterns = []pattern{pat}
	} else {
		for ws.Scan() {
			r.outputs = append(r.outputs, internBytes(ws.Bytes()))
		}
	}

	index++
	if index < len(line) && line[index] == ':' {
		r.isDoubleColon = true
		index++
	}

	rest := line[index:]
	if assign := r.parseVar(rest); assign != nil {
		return assign, nil
	}
	index = bytes.IndexByte(rest, ':')
	if index < 0 {
		r.parseInputs(rest)
		return nil, nil
	}

	// %.x: %.y: %.z
	if isFirstPattern {
		return nil, errors.New("*** mixed implicit and normal rules: deprecated syntax")
	}

	second := rest[:index]
	third := rest[index+1:]

	// r.outputs is already set.
	ws = newWordScanner(second)
	if !ws.Scan() {
		return nil, errors.New("*** missing target pattern.")
	}
	outpat, ok := isPatternRule(ws.Bytes())
	if !ok {
		return nil, errors.New("*** target pattern contains no '%'.")
	}
	r.outputPatterns = []pattern{outpat}
	if ws.Scan() {
		return nil, errors.New("*** multiple target patterns.")
	}
	r.parseInputs(third)

	return nil, nil
}
