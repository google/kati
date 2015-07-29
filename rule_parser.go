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

type rule struct {
	srcpos
	// outputs is output of the rule.
	// []string{} for ': xxx'
	// nil for empty line.
	outputs []string

	inputs          []string
	orderOnlyInputs []string
	outputPatterns  []pattern
	isDoubleColon   bool
	isSuffixRule    bool
	cmds            []string
	cmdLineno       int
}

func (r *rule) cmdpos() srcpos {
	return srcpos{filename: r.filename, lineno: r.cmdLineno}
}

func isPatternRule(s []byte) (pattern, bool) {
	i := findLiteralChar(s, '%', 0, noSkipVar)
	if i < 0 {
		return pattern{}, false
	}
	return pattern{prefix: string(s[:i]), suffix: string(s[i+1:])}, true
}

func unescapeInput(s []byte) []byte {
	// only "\ ", "\=" becoms " ", "=" respectively?
	// other \-escape, such as "\:" keeps "\:".
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			continue
		}
		if i+1 < len(s) && s[i+1] == ' ' || s[i+1] == '=' {
			copy(s[i:], s[i+1:])
			s = s[:len(s)-1]
		}
	}
	return s
}

func unescapeTarget(s []byte) []byte {
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			continue
		}
		copy(s[i:], s[i+1:])
		s = s[:len(s)-1]
	}
	return s
}

func (r *rule) parseInputs(s []byte) {
	ws := newWordScanner(s)
	ws.esc = true
	add := func(t string) {
		r.inputs = append(r.inputs, t)
	}
	for ws.Scan() {
		input := ws.Bytes()
		if len(input) == 1 && input[0] == '|' {
			add = func(t string) {
				r.orderOnlyInputs = append(r.orderOnlyInputs, t)
			}
			continue
		}
		input = unescapeInput(input)
		if !hasWildcardMetaByte(input) {
			add(internBytes(input))
			continue
		}
		m, _ := fsCache.Glob(string(input))
		if len(m) == 0 {
			add(internBytes(input))
			continue
		}
		for _, t := range m {
			add(intern(t))
		}
	}
}

func (r *rule) parseVar(s []byte, rhs expr) (*assignAST, error) {
	var lhsBytes []byte
	var op string
	// TODO(ukai): support override, export.
	if s[len(s)-1] != '=' {
		panic(fmt.Sprintf("unexpected lhs %q", s))
	}
	switch s[len(s)-2] { // s[len(s)-1] is '='
	case ':':
		lhsBytes = trimSpaceBytes(s[:len(s)-2])
		op = ":="
	case '+':
		lhsBytes = trimSpaceBytes(s[:len(s)-2])
		op = "+="
	case '?':
		lhsBytes = trimSpaceBytes(s[:len(s)-2])
		op = "?="
	default:
		lhsBytes = trimSpaceBytes(s[:len(s)-1])
		op = "="
	}
	assign := &assignAST{
		lhs: literal(string(lhsBytes)),
		rhs: compactExpr(rhs),
		op:  op,
	}
	assign.srcpos = r.srcpos
	return assign, nil
}

// parse parses rule line.
// line is rule line until '=', or before ';'.
// line was already expaned, so probably no need to skip var $(xxx) when
// finding literal char. i.e. $ is parsed as literal '$'.
// assign is not nil, if line was known as target specific var '<xxx>: <v>=<val>'
// rhs is not nil, if line ended with '=' (target specific var after evaluated)
func (r *rule) parse(line []byte, assign *assignAST, rhs expr) (*assignAST, error) {
	line = trimLeftSpaceBytes(line)
	// See semicolon.mk.
	if rhs == nil && (len(line) == 0 || line[0] == ';') {
		return nil, nil
	}
	r.outputs = []string{}

	index := findLiteralChar(line, ':', 0, noSkipVar)
	if index < 0 {
		return nil, errors.New("*** missing separator.")
	}

	first := line[:index]
	ws := newWordScanner(first)
	ws.esc = true
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
			// TODO(ukai): expand raw wildcard for output. any usage?
			r.outputs = append(r.outputs, internBytes(unescapeTarget(ws.Bytes())))
		}
	}

	index++
	if index < len(line) && line[index] == ':' {
		r.isDoubleColon = true
		index++
	}

	rest := line[index:]
	if assign != nil {
		if len(rest) > 0 {
			panic(fmt.Sprintf("pattern specific var? line:%q", line))
		}
		return assign, nil
	}
	if rhs != nil {
		assign, err := r.parseVar(rest, rhs)
		if err != nil {
			return nil, err
		}
		return assign, nil
	}
	index = bytes.IndexByte(rest, ';')
	if index >= 0 {
		r.cmds = append(r.cmds, string(rest[index+1:]))
		rest = rest[:index-1]
	}
	index = findLiteralChar(rest, ':', 0, noSkipVar)
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
