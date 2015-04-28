package main

import (
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

func isPatternRule(s string) (pattern, bool) {
	i := strings.IndexByte(s, '%')
	if i < 0 {
		return pattern{}, false
	}
	return pattern{prefix: s[:i], suffix: s[i+1:]}, true
}

func (r *Rule) parseInputs(s string) {
	inputs := splitSpaces(s)
	isOrderOnly := false
	for _, input := range inputs {
		if input == "|" {
			isOrderOnly = true
			continue
		}
		if isOrderOnly {
			r.orderOnlyInputs = append(r.orderOnlyInputs, input)
		} else {
			r.inputs = append(r.inputs, input)
		}
	}
}

func (r *Rule) parseVar(s string) *AssignAST {
	eq := strings.IndexByte(s, '=')
	if eq <= 0 {
		return nil
	}
	assign := &AssignAST{
		rhs: trimLeftSpace(s[eq+1:]),
	}
	assign.filename = r.filename
	assign.lineno = r.lineno
	// TODO(ukai): support override, export.
	switch s[eq-1 : eq+1] {
	case ":=":
		assign.lhs = strings.TrimSpace(s[:eq-1])
		assign.op = ":="
	case "+=":
		assign.lhs = strings.TrimSpace(s[:eq-1])
		assign.op = "+="
	case "?=":
		assign.lhs = strings.TrimSpace(s[:eq-1])
		assign.op = "?="
	default:
		assign.lhs = strings.TrimSpace(s[:eq])
		assign.op = "="
	}
	return assign
}

func (r *Rule) parse(line string) (*AssignAST, error) {
	index := strings.IndexByte(line, ':')
	if index < 0 {
		return nil, errors.New("*** missing separator.")
	}

	first := line[:index]
	outputs := splitSpaces(first)
	pat, isFirstPattern := isPatternRule(first)
	if isFirstPattern {
		if len(outputs) > 1 {
			return nil, errors.New("*** mixed implicit and normal rules: deprecated syntax")
		}
		r.outputPatterns = []pattern{pat}
	} else {
		r.outputs = outputs
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
	index = strings.IndexByte(rest, ':')
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

	r.outputs = outputs
	outputPatterns := splitSpaces(second)
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
