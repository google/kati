package main

import (
	"strings"
)

type Rule struct {
	outputs   []string
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

	r.outputs = splitSpaces(line[:colonIndex])
	r.inputs = splitSpaces(line[colonIndex+1:])
}
