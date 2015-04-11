package main

import (
	"errors"
	"strings"
)

type Rule struct {
	outputs         []string
	inputs          []string
	orderOnlyInputs []string
	outputPatterns  []string
	isDoubleColon   bool
	isSuffixRule    bool
	vars            Vars
	cmds            []string
	filename        string
	lineno          int
	cmdLineno       int
}

func isPatternRule(s string) bool {
	return strings.IndexByte(s, '%') >= 0
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
	isFirstPattern := isPatternRule(first)
	if isFirstPattern {
		if len(outputs) > 1 {
			return nil, errors.New("*** mixed implicit and normal rules: deprecated syntax")
		}
		r.outputPatterns = outputs
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
	r.outputPatterns = splitSpaces(second)
	if len(r.outputPatterns) == 0 {
		return nil, errors.New("*** missing target pattern.")
	}
	if len(r.outputPatterns) > 1 {
		return nil, errors.New("*** multiple target patterns.")
	}
	if !isPatternRule(r.outputPatterns[0]) {
		return nil, errors.New("*** target pattern contains no '%'.")
	}
	r.parseInputs(third)

	return nil, nil
}
