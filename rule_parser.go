package main

import (
	"strings"
)

type Rule struct {
	outputs        []string
	inputs         []string
	outputPatterns []string
	isDoubleColon  bool
	isSuffixRule   bool
	cmds           []string
	filename       string
	lineno         int
	cmdLineno      int
}

func isPatternRule(s string) bool {
	return strings.IndexByte(s, '%') >= 0
}

func (r *Rule) parse(line string) string {
	index := strings.IndexByte(line, ':')
	if index < 0 {
		return "*** missing separator."
	}

	first := line[:index]
	outputs := splitSpaces(first)
	isFirstPattern := isPatternRule(first)
	if isFirstPattern {
		if len(outputs) > 1 {
			return "*** mixed implicit and normal rules: deprecated syntax"
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
	index = strings.IndexByte(rest, ':')
	if index < 0 {
		r.inputs = splitSpaces(rest)
		return ""
	}

	// %.x: %.y: %.z
	if isFirstPattern {
		return "*** mixed implicit and normal rules: deprecated syntax"
	}

	second := rest[:index]
	third := rest[index+1:]

	r.outputs = outputs
	r.outputPatterns = splitSpaces(second)
	if len(r.outputPatterns) == 0 {
		return "*** missing target pattern."
	}
	if len(r.outputPatterns) > 1 {
		return "*** multiple target patterns."
	}
	if !isPatternRule(r.outputPatterns[0]) {
		return "*** target pattern contains no '%'."
	}
	r.inputs = splitSpaces(third)

	return ""
}
