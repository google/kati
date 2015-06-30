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

//go:generate go run testcase/gen_testcase_parse_benchmark.go
//
// $ go generate
// $ go test -bench .

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"sync"
	"time"
)

type makefile struct {
	filename string
	stmts    []ast
}

type ifState struct {
	ast     *ifAST
	inElse  bool
	numNest int
}

type parser struct {
	rd          *bufio.Reader
	mk          makefile
	lineno      int
	elineno     int // lineno == elineno unless there is trailing '\'.
	linenoFixed bool
	done        bool
	outStmts    *[]ast
	ifStack     []ifState
	inDef       []string
	defOpt      string
	numIfNest   int
	err         error
}

func newParser(rd io.Reader, filename string) *parser {
	p := &parser{
		rd: bufio.NewReader(rd),
	}
	p.mk.filename = filename
	p.outStmts = &p.mk.stmts
	return p
}

func (p *parser) srcpos() srcpos {
	return srcpos{
		filename: p.mk.filename,
		lineno:   p.lineno,
	}
}

func (p *parser) addStatement(stmt ast) {
	*p.outStmts = append(*p.outStmts, stmt)
}

func (p *parser) readLine() []byte {
	if !p.linenoFixed {
		p.lineno = p.elineno
	}
	line, err := p.rd.ReadBytes('\n')
	if !p.linenoFixed {
		p.lineno++
		p.elineno = p.lineno
	}
	if err == io.EOF {
		p.done = true
	} else if err != nil {
		p.err = fmt.Errorf("readline %s: %v", p.srcpos(), err)
		p.done = true
	}

	line = bytes.TrimRight(line, "\r\n")

	return line
}

func removeComment(line []byte) []byte {
	var parenStack []byte
	// Do not use range as we may modify |line| and |i|.
	for i := 0; i < len(line); i++ {
		ch := line[i]
		switch ch {
		case '(', '{':
			parenStack = append(parenStack, ch)
		case ')', '}':
			if len(parenStack) > 0 {
				cp := closeParen(parenStack[len(parenStack)-1])
				if cp == ch {
					parenStack = parenStack[:len(parenStack)-1]
				}
			}
		case '#':
			if len(parenStack) == 0 {
				if i == 0 || line[i-1] != '\\' {
					return line[:i]
				}
				// Drop the backslash before '#'.
				line = append(line[:i-1], line[i:]...)
				i--
			}
		}
	}
	return line
}

func hasTrailingBackslash(line []byte) bool {
	if len(line) == 0 {
		return false
	}
	if line[len(line)-1] != '\\' {
		return false
	}
	return len(line) <= 1 || line[len(line)-2] != '\\'
}

func (p *parser) processDefineLine(line []byte) []byte {
	for hasTrailingBackslash(line) {
		line = line[:len(line)-1]
		line = bytes.TrimRight(line, "\t ")
		lineno := p.lineno
		nline := trimLeftSpaceBytes(p.readLine())
		p.lineno = lineno
		line = append(line, ' ')
		line = append(line, nline...)
	}
	return line
}

func (p *parser) processMakefileLine(line []byte) []byte {
	return removeComment(p.processDefineLine(line))
}

func (p *parser) processRecipeLine(line []byte) []byte {
	for hasTrailingBackslash(line) {
		line = append(line, '\n')
		lineno := p.lineno
		nline := p.readLine()
		p.lineno = lineno
		line = append(line, nline...)
	}
	return line
}

func newAssignAST(p *parser, lhsBytes []byte, rhsBytes []byte, op string) (*assignAST, error) {
	lhs, _, err := parseExpr(lhsBytes, nil, true)
	if err != nil {
		return nil, err
	}
	rhs, _, err := parseExpr(rhsBytes, nil, true)
	if err != nil {
		return nil, err
	}
	opt := ""
	if p != nil {
		opt = p.defOpt
	}
	return &assignAST{
		lhs: lhs,
		rhs: rhs,
		op:  op,
		opt: opt,
	}, nil
}

func (p *parser) parseAssign(line []byte, sep, esep int) (ast, error) {
	logf("parseAssign %q op:%q", line, line[sep:esep])
	aast, err := newAssignAST(p, bytes.TrimSpace(line[:sep]), trimLeftSpaceBytes(line[esep:]), string(line[sep:esep]))
	if err != nil {
		return nil, err
	}
	aast.srcpos = p.srcpos()
	return aast, nil
}

func (p *parser) parseMaybeRule(line []byte, equalIndex, semicolonIndex int) (ast, error) {
	if len(trimSpaceBytes(line)) == 0 {
		return nil, nil
	}

	expr := line
	var term byte
	var afterTerm []byte

	// Either '=' or ';' is used.
	if equalIndex >= 0 && semicolonIndex >= 0 {
		if equalIndex < semicolonIndex {
			semicolonIndex = -1
		} else {
			equalIndex = -1
		}
	}
	if semicolonIndex >= 0 {
		afterTerm = expr[semicolonIndex:]
		expr = expr[0:semicolonIndex]
		term = ';'
	} else if equalIndex >= 0 {
		afterTerm = expr[equalIndex:]
		expr = expr[0:equalIndex]
		term = '='
	}

	v, _, err := parseExpr(expr, nil, true)
	if err != nil {
		return nil, p.srcpos().error(err)
	}

	rast := &maybeRuleAST{
		expr:      v,
		term:      term,
		afterTerm: afterTerm,
	}
	rast.srcpos = p.srcpos()
	return rast, nil
}

func (p *parser) parseInclude(line string, oplen int) {
	// TODO(ukai): parse expr here
	iast := &includeAST{
		expr: line[oplen+1:],
		op:   line[:oplen],
	}
	iast.srcpos = p.srcpos()
	p.addStatement(iast)
}

func (p *parser) parseIfdef(line []byte, oplen int) {
	lhs, _, err := parseExpr(trimLeftSpaceBytes(line[oplen+1:]), nil, true)
	if err != nil {
		p.err = p.srcpos().error(err)
		return
	}
	iast := &ifAST{
		op:  string(line[:oplen]),
		lhs: lhs,
	}
	iast.srcpos = p.srcpos()
	p.addStatement(iast)
	p.ifStack = append(p.ifStack, ifState{ast: iast, numNest: p.numIfNest})
	p.outStmts = &iast.trueStmts
}

func (p *parser) parseTwoQuotes(s string, op string) ([]string, bool, error) {
	var args []string
	for i := 0; i < 2; i++ {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, false, nil
		}
		quote := s[0]
		if quote != '\'' && quote != '"' {
			return nil, false, nil
		}
		end := strings.IndexByte(s[1:], quote) + 1
		if end < 0 {
			return nil, false, nil
		}
		args = append(args, s[1:end])
		s = s[end+1:]
	}
	if len(s) > 0 {
		return nil, false, p.srcpos().errorf(`extraneous text after %q directive`, op)
	}
	return args, true, nil
}

// parse
//  "(lhs, rhs)"
//  "lhs, rhs"
func (p *parser) parseEq(s string, op string) (string, string, bool, error) {
	if s[0] == '(' && s[len(s)-1] == ')' {
		s = s[1 : len(s)-1]
		term := []byte{','}
		in := []byte(s)
		v, n, err := parseExpr(in, term, false)
		if err != nil {
			return "", "", false, err
		}
		lhs := v.String()
		n++
		n += skipSpaces(in[n:], nil)
		v, n, err = parseExpr(in[n:], nil, false)
		if err != nil {
			return "", "", false, err
		}
		rhs := v.String()
		return lhs, rhs, true, nil
	}
	args, ok, err := p.parseTwoQuotes(s, op)
	if !ok {
		return "", "", false, err
	}
	return args[0], args[1], true, nil
}

func (p *parser) parseIfeq(line string, oplen int) {
	op := line[:oplen]
	lhsBytes, rhsBytes, ok, err := p.parseEq(strings.TrimSpace(line[oplen+1:]), op)
	if err != nil {
		p.err = err
		return
	}
	if !ok {
		p.err = p.srcpos().errorf(`*** invalid syntax in conditional.`)
		return
	}

	lhs, _, err := parseExpr([]byte(lhsBytes), nil, true)
	if err != nil {
		p.err = p.srcpos().error(err)
		return
	}
	rhs, _, err := parseExpr([]byte(rhsBytes), nil, true)
	if err != nil {
		p.err = p.srcpos().error(err)
		return
	}

	iast := &ifAST{
		op:  op,
		lhs: lhs,
		rhs: rhs,
	}
	iast.srcpos = p.srcpos()
	p.addStatement(iast)
	p.ifStack = append(p.ifStack, ifState{ast: iast, numNest: p.numIfNest})
	p.outStmts = &iast.trueStmts
	return
}

func (p *parser) checkIfStack(curKeyword string) error {
	if len(p.ifStack) == 0 {
		return p.srcpos().errorf(`*** extraneous %q.`, curKeyword)
	}
	return nil
}

func (p *parser) parseElse(line []byte) {
	err := p.checkIfStack("else")
	if err != nil {
		p.err = err
		return
	}
	state := &p.ifStack[len(p.ifStack)-1]
	if state.inElse {
		p.err = p.srcpos().errorf(`*** only one "else" per conditional.`)
		return
	}
	state.inElse = true
	p.outStmts = &state.ast.falseStmts

	nextIf := trimLeftSpaceBytes(line[len("else"):])
	if len(nextIf) == 0 {
		return
	}
	var ifDirectives = map[string]directiveFunc{
		"ifdef ":  ifdefDirective,
		"ifndef ": ifndefDirective,
		"ifeq ":   ifeqDirective,
		"ifneq ":  ifneqDirective,
	}
	p.numIfNest = state.numNest + 1
	if f, ok := p.isDirective(nextIf, ifDirectives); ok {
		f(p, nextIf)
		p.numIfNest = 0
		return
	}
	p.numIfNest = 0
	warnNoPrefix(p.srcpos(), "extraneous text after `else` directive")
	return
}

func (p *parser) parseEndif(line string) {
	err := p.checkIfStack("endif")
	if err != nil {
		p.err = err
		return
	}
	state := p.ifStack[len(p.ifStack)-1]
	for t := 0; t <= state.numNest; t++ {
		p.ifStack = p.ifStack[0 : len(p.ifStack)-1]
		if len(p.ifStack) == 0 {
			p.outStmts = &p.mk.stmts
		} else {
			state := p.ifStack[len(p.ifStack)-1]
			if state.inElse {
				p.outStmts = &state.ast.falseStmts
			} else {
				p.outStmts = &state.ast.trueStmts
			}
		}
	}
	return
}

type directiveFunc func(*parser, []byte) []byte

var makeDirectives = map[string]directiveFunc{
	"include ":  includeDirective,
	"-include ": sincludeDirective,
	"sinclude":  sincludeDirective,
	"ifdef ":    ifdefDirective,
	"ifndef ":   ifndefDirective,
	"ifeq ":     ifeqDirective,
	"ifneq ":    ifneqDirective,
	"else":      elseDirective,
	"endif":     endifDirective,
	"define ":   defineDirective,
	"override ": overrideDirective,
	"export ":   exportDirective,
	"unexport ": unexportDirective,
}

// TODO(ukai): use []byte
func (p *parser) isDirective(line []byte, directives map[string]directiveFunc) (directiveFunc, bool) {
	stripped := trimLeftSpaceBytes(line)
	// Fast paths.
	// TODO: Consider using a trie.
	if len(stripped) == 0 {
		return nil, false
	}
	if ch := stripped[0]; ch != 'i' && ch != '-' && ch != 's' && ch != 'e' && ch != 'd' && ch != 'o' && ch != 'u' {
		return nil, false
	}

	for prefix, f := range directives {
		if bytes.HasPrefix(stripped, []byte(prefix)) {
			return f, true
		}
		if prefix[len(prefix)-1] == ' ' && bytes.HasPrefix(stripped, []byte(prefix[:len(prefix)-1])) && len(stripped) >= len(prefix) && stripped[len(prefix)-1] == '\t' {
			return f, true
		}
	}
	return nil, false
}

func includeDirective(p *parser, line []byte) []byte {
	p.parseInclude(string(line), len("include"))
	return nil
}

func sincludeDirective(p *parser, line []byte) []byte {
	p.parseInclude(string(line), len("-include"))
	return nil
}

func ifdefDirective(p *parser, line []byte) []byte {
	p.parseIfdef(line, len("ifdef"))
	return nil
}

func ifndefDirective(p *parser, line []byte) []byte {
	p.parseIfdef(line, len("ifndef"))
	return nil
}

func ifeqDirective(p *parser, line []byte) []byte {
	p.parseIfeq(string(line), len("ifeq"))
	return nil
}

func ifneqDirective(p *parser, line []byte) []byte {
	p.parseIfeq(string(line), len("ifneq"))
	return nil
}

func elseDirective(p *parser, line []byte) []byte {
	p.parseElse(line)
	return nil
}

func endifDirective(p *parser, line []byte) []byte {
	p.parseEndif(string(line))
	return nil
}

func defineDirective(p *parser, line []byte) []byte {
	lhs := trimLeftSpaceBytes(line[len("define "):])
	p.inDef = []string{string(lhs)}
	return nil
}

func overrideDirective(p *parser, line []byte) []byte {
	p.defOpt = "override"
	line = trimLeftSpaceBytes(line[len("override "):])
	defineDirective := map[string]directiveFunc{
		"define": defineDirective,
	}
	if f, ok := p.isDirective(line, defineDirective); ok {
		f(p, line)
		return nil
	}
	// e.g. overrider foo := bar
	// line will be "foo := bar".
	return line
}

func handleExport(p *parser, line []byte, export bool) (hasEqual bool) {
	equalIndex := bytes.IndexByte(line, '=')
	if equalIndex > 0 {
		hasEqual = true
		switch line[equalIndex-1] {
		case ':', '+', '?':
			equalIndex--
		}
		line = line[:equalIndex]
	}

	east := &exportAST{
		expr:   line,
		export: export,
	}
	east.srcpos = p.srcpos()
	p.addStatement(east)
	return hasEqual
}

func exportDirective(p *parser, line []byte) []byte {
	p.defOpt = "export"
	line = trimLeftSpaceBytes(line[len("export "):])
	defineDirective := map[string]directiveFunc{
		"define": defineDirective,
	}
	if f, ok := p.isDirective(line, defineDirective); ok {
		f(p, line)
		return nil
	}

	if !handleExport(p, line, true) {
		return nil
	}

	// e.g. export foo := bar
	// line will be "foo := bar".
	return line
}

func unexportDirective(p *parser, line []byte) []byte {
	handleExport(p, line[len("unexport "):], false)
	return nil
}

func (p *parser) isEndef(s string) bool {
	if s == "endef" {
		return true
	}
	found := strings.IndexAny(s, " \t")
	if found >= 0 && s[:found] == "endef" {
		rest := strings.TrimSpace(s[found+1:])
		if rest != "" && rest[0] != '#' {
			warnNoPrefix(p.srcpos(), "extraneous text after \"endef\" directive")
		}
		return true
	}
	return false
}

func (p *parser) parse() (mk makefile, err error) {
	for !p.done {
		line := p.readLine()

		if len(p.inDef) > 0 {
			lineStr := string(p.processDefineLine(line))
			if p.isEndef(lineStr) {
				logf("multilineAssign %q", p.inDef)
				aast, err := newAssignAST(p, []byte(p.inDef[0]), []byte(strings.Join(p.inDef[1:], "\n")), "=")
				if err != nil {
					return makefile{}, err
				}
				aast.srcpos = p.srcpos()
				aast.srcpos.lineno -= len(p.inDef)
				p.addStatement(aast)
				p.inDef = nil
				p.defOpt = ""
				continue
			}
			p.inDef = append(p.inDef, lineStr)
			continue
		}
		p.defOpt = ""

		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		if f, ok := p.isDirective(line, makeDirectives); ok {
			line = trimSpaceBytes(p.processMakefileLine(line))
			line = f(p, line)
			if p.err != nil {
				return makefile{}, p.err
			}
			if len(line) == 0 {
				continue
			}
		}
		if line[0] == '\t' {
			cast := &commandAST{cmd: string(p.processRecipeLine(line[1:]))}
			cast.srcpos = p.srcpos()
			p.addStatement(cast)
			continue
		}

		line = p.processMakefileLine(line)

		var stmt ast
		var parenStack []byte
		equalIndex := -1
		semicolonIndex := -1
		isRule := false
		for i, ch := range line {
			switch ch {
			case '(', '{':
				parenStack = append(parenStack, ch)
			case ')', '}':
				if len(parenStack) == 0 {
					warn(p.srcpos(), "Unmatched parens: %s", line)
				} else {
					cp := closeParen(parenStack[len(parenStack)-1])
					if cp == ch {
						parenStack = parenStack[:len(parenStack)-1]
					}
				}
			}
			if len(parenStack) > 0 {
				continue
			}

			switch ch {
			case ':':
				if i+1 < len(line) && line[i+1] == '=' {
					if !isRule {
						stmt, err = p.parseAssign(line, i, i+2)
						if err != nil {
							return makefile{}, err
						}
					}
				} else {
					isRule = true
				}
			case ';':
				if semicolonIndex < 0 {
					semicolonIndex = i
				}
			case '=':
				if !isRule {
					stmt, err = p.parseAssign(line, i, i+1)
					if err != nil {
						return makefile{}, err
					}
				}
				if equalIndex < 0 {
					equalIndex = i
				}
			case '?', '+':
				if !isRule && i+1 < len(line) && line[i+1] == '=' {
					stmt, err = p.parseAssign(line, i, i+2)
					if err != nil {
						return makefile{}, err
					}
				}
			}
			if stmt != nil {
				p.addStatement(stmt)
				break
			}
		}
		if stmt == nil {
			stmt, err = p.parseMaybeRule(line, equalIndex, semicolonIndex)
			if err != nil {
				return makefile{}, err
			}
			if stmt != nil {
				p.addStatement(stmt)
			}
		}
	}
	return p.mk, p.err
}

func defaultMakefile() (string, error) {
	candidates := []string{"GNUmakefile", "makefile", "Makefile"}
	for _, filename := range candidates {
		if exists(filename) {
			return filename, nil
		}
	}
	return "", errors.New("no targets specified and no makefile found")
}

func parseMakefileReader(rd io.Reader, loc srcpos) (makefile, error) {
	parser := newParser(rd, loc.filename)
	parser.lineno = loc.lineno
	parser.elineno = loc.lineno
	parser.linenoFixed = true
	return parser.parse()
}

func parseMakefileString(s string, loc srcpos) (makefile, error) {
	return parseMakefileReader(strings.NewReader(s), loc)
}

func parseMakefileBytes(s []byte, loc srcpos) (makefile, error) {
	return parseMakefileReader(bytes.NewReader(s), loc)
}

type mkCacheEntry struct {
	mk   makefile
	hash [sha1.Size]byte
	err  error
	ts   int64
}

type makefileCacheT struct {
	mu sync.Mutex
	mk map[string]mkCacheEntry
}

var makefileCache = &makefileCacheT{
	mk: make(map[string]mkCacheEntry),
}

func (mc *makefileCacheT) lookup(filename string) (makefile, [sha1.Size]byte, bool, error) {
	var hash [sha1.Size]byte
	mc.mu.Lock()
	c, present := mc.mk[filename]
	mc.mu.Unlock()
	if !present {
		return makefile{}, hash, false, nil
	}
	ts := getTimestamp(filename)
	if ts < 0 || ts >= c.ts {
		return makefile{}, hash, false, nil
	}
	return c.mk, c.hash, true, c.err
}

func (mc *makefileCacheT) parse(filename string) (makefile, [sha1.Size]byte, error) {
	logf("parse Makefile %q", filename)
	mk, hash, ok, err := makefileCache.lookup(filename)
	if ok {
		if LogFlag {
			logf("makefile cache hit for %q", filename)
		}
		return mk, hash, err
	}
	if LogFlag {
		logf("reading makefile %q", filename)
	}
	c, err := ioutil.ReadFile(filename)
	if err != nil {
		return makefile{}, hash, err
	}
	hash = sha1.Sum(c)
	mk, err = parseMakefile(c, filename)
	if err != nil {
		return makefile{}, hash, err
	}
	makefileCache.mu.Lock()
	makefileCache.mk[filename] = mkCacheEntry{
		mk:   mk,
		hash: hash,
		err:  err,
		ts:   time.Now().Unix(),
	}
	makefileCache.mu.Unlock()
	return mk, hash, err
}

func parseMakefile(s []byte, filename string) (makefile, error) {
	parser := newParser(bytes.NewReader(s), filename)
	return parser.parse()
}
