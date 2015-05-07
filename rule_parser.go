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
	s = trimSpaceBytes(s)
	i := bytes.IndexByte(s, '%')
	if i < 0 {
		return pattern{}, false
	}
	return pattern{prefix: string(s[:i]), suffix: string(s[i+1:])}, true
}

func (r *Rule) parseInputs(s []byte) {
	inputs := splitSpacesBytes(s)
	isOrderOnly := false
	for _, input := range inputs {
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
	assign := &AssignAST{
		rhs: string(trimLeftSpaceBytes(s[eq+1:])),
	}
	assign.filename = r.filename
	assign.lineno = r.lineno
	// TODO(ukai): support override, export.
	switch s[eq-1] { // s[eq] is '='
	case ':':
		assign.lhs = string(trimSpaceBytes(s[:eq-1]))
		assign.op = ":="
	case '+':
		assign.lhs = string(trimSpaceBytes(s[:eq-1]))
		assign.op = "+="
	case '?':
		assign.lhs = string(trimSpaceBytes(s[:eq-1]))
		assign.op = "?="
	default:
		assign.lhs = string(trimSpaceBytes(s[:eq]))
		assign.op = "="
	}
	return assign
}

func (r *Rule) parse(line []byte) (*AssignAST, error) {
	index := bytes.IndexByte(line, ':')
	if index < 0 {
		return nil, errors.New("*** missing separator.")
	}

	first := line[:index]
	outputs := splitSpacesBytes(first)
	pat, isFirstPattern := isPatternRule(first)
	if isFirstPattern {
		if len(outputs) > 1 {
			return nil, errors.New("*** mixed implicit and normal rules: deprecated syntax")
		}
		r.outputPatterns = []pattern{pat}
	} else {
		o := make([]string, len(outputs))
		for i, output := range outputs {
			o[i] = internBytes(output)
		}
		r.outputs = o
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
	outputPatterns := splitSpacesBytes(second)
	if len(outputPatterns) == 0 {
		return nil, errors.New("*** missing target pattern.")
	}
	if len(outputPatterns) > 1 {
		return nil, errors.New("*** multiple target patterns.")
	}
	outpat, ok := isPatternRule(outputPatterns[0])
	if !ok {
		return nil, errors.New("*** target pattern contains no '%'.")
	}
	r.outputPatterns = []pattern{outpat}
	r.parseInputs(third)

	return nil, nil
}
