package main

import (
	"strings"
)

type Rule struct {
	output    string
	inputs    []string
	cmds      []string
	filename  string
	lineno    int
	cmdLineno int
}

func (r *Rule) parse(line string) {
	colonIndex := strings.IndexByte(line, ':')
	if colonIndex < 0 {
		Error(r.filename, r.lineno, "*** missing separator.")
	}

	lhs := line[:colonIndex]
	r.output = lhs
	r.inputs = splitSpaces(line[colonIndex+1:])
}
