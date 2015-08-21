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

	"github.com/golang/glog"
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
	inRecipe    bool
	ifStack     []ifState

	defineVar []byte
	inDef     []byte

	defOpt    string
	numIfNest int
	err       error
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
	switch stmt.(type) {
	case *maybeRuleAST:
		p.inRecipe = true
	case *assignAST, *includeAST, *exportAST:
		p.inRecipe = false
	}
}

func (p *parser) readLine() []byte {
	if !p.linenoFixed {
		p.lineno = p.elineno + 1
	}
	var line []byte
	for !p.done {
		buf, err := p.rd.ReadBytes('\n')
		if !p.linenoFixed {
			p.elineno++
		}
		if err == io.EOF {
			p.done = true
		} else if err != nil {
			p.err = fmt.Errorf("readline %s: %v", p.srcpos(), err)
			p.done = true
		}
		line = append(line, buf...)
		buf = bytes.TrimRight(buf, "\r\n")
		glog.V(4).Infof("buf:%q", buf)
		backslash := false
		for len(buf) > 0 && buf[len(buf)-1] == '\\' {
			buf = buf[:len(buf)-1]
			backslash = !backslash
		}
		if !backslash {
			glog.V(4).Infof("no concat line:%q", buf)
			break
		}
	}
	line = bytes.TrimRight(line, "\r\n")
	return line
}

func newAssignAST(p *parser, lhsBytes []byte, rhsBytes []byte, op string) (*assignAST, error) {
	lhs, _, err := parseExpr(lhsBytes, nil, parseOp{alloc: true})
	if err != nil {
		return nil, err
	}
	rhs, _, err := parseExpr(rhsBytes, nil, parseOp{alloc: true})
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

func (p *parser) handleDirective(line []byte, directives map[string]directiveFunc) bool {
	w, data := firstWord(line)
	if d, ok := directives[string(w)]; ok {
		d(p, data)
		return true
	}
	return false
}

func (p *parser) handleRuleOrAssign(line []byte) {
	rline := line
	var semi []byte
	if i := findLiteralChar(line, ';', 0, skipVar); i >= 0 {
		// preserve after semicolon
		semi = append(semi, line[i+1:]...)
		rline = concatline(line[:i])
	} else {
		rline = concatline(line)
	}
	if p.handleAssign(line) {
		return
	}
	// not assignment.
	// ie. no '=' found or ':' found before '=' (except ':=')
	p.parseMaybeRule(rline, semi)
	return
}

func (p *parser) handleAssign(line []byte) bool {
	aline, _ := removeComment(concatline(line))
	aline = trimLeftSpaceBytes(aline)
	if len(aline) == 0 {
		return false
	}
	// fmt.Printf("assign: %q=>%q\n", line, aline)
	i := findLiteralChar(aline, ':', '=', skipVar)
	if i >= 0 {
		if aline[i] == '=' {
			p.parseAssign(aline, i)
			return true
		}
		if aline[i] == ':' && i+1 < len(aline) && aline[i+1] == '=' {
			p.parseAssign(aline, i+1)
			return true
		}
	}
	return false
}

func (p *parser) parseAssign(line []byte, sep int) {
	lhs, op, rhs := line[:sep], line[sep:sep+1], line[sep+1:]
	if sep > 0 {
		switch line[sep-1] {
		case ':', '+', '?':
			lhs, op = line[:sep-1], line[sep-1:sep+1]
		}
	}
	glog.V(1).Infof("parseAssign %s op:%q opt:%s", line, op, p.defOpt)
	lhs = trimSpaceBytes(lhs)
	rhs = trimLeftSpaceBytes(rhs)
	aast, err := newAssignAST(p, lhs, rhs, string(op))
	if err != nil {
		p.err = err
		return
	}
	aast.srcpos = p.srcpos()
	p.addStatement(aast)
}

func (p *parser) parseMaybeRule(line, semi []byte) {
	if len(line) == 0 {
		p.err = p.srcpos().errorf("*** missing rule before commands.")
		return
	}
	if line[0] == '\t' {
		p.err = p.srcpos().errorf("*** commands commence before first target.")
		return
	}
	var assign *assignAST
	ci := findLiteralChar(line, ':', 0, skipVar)
	if ci >= 0 {
		eqi := findLiteralChar(line[ci+1:], '=', 0, skipVar)
		if eqi == 0 {
			panic(fmt.Sprintf("unexpected eq after colon: %q", line))
		}
		if eqi > 0 {
			var lhsbytes []byte
			op := "="
			switch line[ci+1+eqi-1] {
			case ':', '+', '?':
				lhsbytes = append(lhsbytes, line[ci+1:ci+1+eqi-1]...)
				op = string(line[ci+1+eqi-1 : ci+1+eqi+1])
			default:
				lhsbytes = append(lhsbytes, line[ci+1:ci+1+eqi]...)
			}

			lhsbytes = trimSpaceBytes(lhsbytes)
			lhs, _, err := parseExpr(lhsbytes, nil, parseOp{})
			if err != nil {
				p.err = p.srcpos().error(err)
				return
			}
			var rhsbytes []byte
			rhsbytes = append(rhsbytes, line[ci+1+eqi+1:]...)
			if semi != nil {
				rhsbytes = append(rhsbytes, ';')
				rhsbytes = append(rhsbytes, concatline(semi)...)
			}
			rhsbytes = trimLeftSpaceBytes(rhsbytes)
			semi = nil
			rhs, _, err := parseExpr(rhsbytes, nil, parseOp{})
			if err != nil {
				p.err = p.srcpos().error(err)
				return
			}

			// TODO(ukai): support override, export in target specific var.
			assign = &assignAST{
				lhs: lhs,
				rhs: rhs,
				op:  op,
			}
			assign.srcpos = p.srcpos()
			line = line[:ci+1]
		}
	}
	expr, _, err := parseExpr(line, nil, parseOp{})
	if err != nil {
		p.err = p.srcpos().error(err)
		return
	}
	// TODO(ukai): remove ast, and eval here.
	rast := &maybeRuleAST{
		isRule: ci >= 0,
		expr:   expr,
		assign: assign,
		semi:   semi,
	}
	rast.srcpos = p.srcpos()
	glog.V(1).Infof("stmt: %#v", rast)
	p.addStatement(rast)
}

func (p *parser) parseInclude(op string, line []byte) {
	// TODO(ukai): parse expr here
	iast := &includeAST{
		expr: string(line),
		op:   op,
	}
	iast.srcpos = p.srcpos()
	p.addStatement(iast)
}

func (p *parser) parseIfdef(op string, data []byte) {
	lhs, _, err := parseExpr(data, nil, parseOp{alloc: true})
	if err != nil {
		p.err = p.srcpos().error(err)
		return
	}
	iast := &ifAST{
		op:  op,
		lhs: lhs,
	}
	iast.srcpos = p.srcpos()
	p.addStatement(iast)
	p.ifStack = append(p.ifStack, ifState{ast: iast, numNest: p.numIfNest})
	p.outStmts = &iast.trueStmts
}

func (p *parser) parseTwoQuotes(s []byte) (string, string, []byte, bool) {
	var args []string
	for i := 0; i < 2; i++ {
		s = trimSpaceBytes(s)
		if len(s) == 0 {
			return "", "", nil, false
		}
		quote := s[0]
		if quote != '\'' && quote != '"' {
			return "", "", nil, false
		}
		end := bytes.IndexByte(s[1:], quote) + 1
		if end < 0 {
			return "", "", nil, false
		}
		args = append(args, string(s[1:end]))
		s = s[end+1:]
	}
	return args[0], args[1], s, true
}

// parse
//  "(lhs, rhs)"
//  "lhs, rhs"
func (p *parser) parseEq(s []byte) (string, string, []byte, bool) {
	if len(s) == 0 {
		return "", "", nil, false
	}
	if s[0] == '(' {
		in := s[1:]
		glog.V(1).Infof("parseEq ( %q )", in)
		term := []byte{','}
		v, n, err := parseExpr(in, term, parseOp{matchParen: true})
		if err != nil {
			glog.V(1).Infof("parse eq: %q: %v", in, err)
			return "", "", nil, false
		}
		lhs := v.String()
		n++
		n += skipSpaces(in[n:], nil)
		term = []byte{')'}
		in = in[n:]
		v, n, err = parseExpr(in, term, parseOp{matchParen: true})
		if err != nil {
			glog.V(1).Infof("parse eq 2nd: %q: %v", in, err)
			return "", "", nil, false
		}
		rhs := v.String()
		in = in[n+1:]
		in = trimSpaceBytes(in)
		return lhs, rhs, in, true
	}
	return p.parseTwoQuotes(s)
}

func (p *parser) parseIfeq(op string, data []byte) {
	lhsBytes, rhsBytes, extra, ok := p.parseEq(data)
	if !ok {
		p.err = p.srcpos().errorf(`*** invalid syntax in conditional.`)
		return
	}
	if len(extra) > 0 {
		glog.V(1).Infof("extra %q", extra)
		p.err = p.srcpos().errorf(`extraneous text after %q directive`, op)
		return
	}

	lhs, _, err := parseExpr([]byte(lhsBytes), nil, parseOp{matchParen: true})
	if err != nil {
		p.err = p.srcpos().error(err)
		return
	}
	rhs, _, err := parseExpr([]byte(rhsBytes), nil, parseOp{matchParen: true})
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
}

func (p *parser) checkIfStack(curKeyword string) error {
	if len(p.ifStack) == 0 {
		return p.srcpos().errorf(`*** extraneous %q.`, curKeyword)
	}
	return nil
}

func (p *parser) parseElse(data []byte) {
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

	nextIf := data
	if len(nextIf) == 0 {
		return
	}
	var ifDirectives = map[string]directiveFunc{
		"ifdef":  ifdefDirective,
		"ifndef": ifndefDirective,
		"ifeq":   ifeqDirective,
		"ifneq":  ifneqDirective,
	}
	p.numIfNest = state.numNest + 1
	if p.handleDirective(nextIf, ifDirectives) {
		p.numIfNest = 0
		return
	}
	p.numIfNest = 0
	warnNoPrefix(p.srcpos(), "extraneous text after `else' directive")
	return
}

func (p *parser) parseEndif(data []byte) {
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
	if len(trimSpaceBytes(data)) > 0 {
		warnNoPrefix(p.srcpos(), "extraneous text after `endif' directive")
	}
	return
}

func (p *parser) parseDefine(data []byte) {
	p.defineVar = nil
	p.inDef = nil
	p.defineVar = append(p.defineVar, trimSpaceBytes(data)...)
	return
}

func (p *parser) parseVpath(data []byte) {
	vline, _ := removeComment(concatline(data))
	vline = trimLeftSpaceBytes(vline)
	v, _, err := parseExpr(vline, nil, parseOp{})
	if err != nil {
		p.err = p.srcpos().errorf("parse error %q: %v", string(vline), err)
		return
	}
	vast := &vpathAST{
		expr: v,
	}
	vast.srcpos = p.srcpos()
	p.addStatement(vast)
}

type directiveFunc func(*parser, []byte)

var makeDirectives map[string]directiveFunc

func init() {
	makeDirectives = map[string]directiveFunc{
		"include":  includeDirective,
		"-include": sincludeDirective,
		"sinclude": sincludeDirective,
		"ifdef":    ifdefDirective,
		"ifndef":   ifndefDirective,
		"ifeq":     ifeqDirective,
		"ifneq":    ifneqDirective,
		"else":     elseDirective,
		"endif":    endifDirective,
		"define":   defineDirective,
		"override": overrideDirective,
		"export":   exportDirective,
		"unexport": unexportDirective,
		"vpath":    vpathDirective,
	}
}

func includeDirective(p *parser, data []byte) {
	p.parseInclude("include", data)
}

func sincludeDirective(p *parser, data []byte) {
	p.parseInclude("-include", data)
}

func ifdefDirective(p *parser, data []byte) {
	p.parseIfdef("ifdef", data)
}

func ifndefDirective(p *parser, data []byte) {
	p.parseIfdef("ifndef", data)
}

func ifeqDirective(p *parser, data []byte) {
	p.parseIfeq("ifeq", data)
}

func ifneqDirective(p *parser, data []byte) {
	p.parseIfeq("ifneq", data)
}

func elseDirective(p *parser, data []byte) {
	p.parseElse(data)
}

func endifDirective(p *parser, data []byte) {
	p.parseEndif(data)
}

func defineDirective(p *parser, data []byte) {
	p.parseDefine(data)
}

func overrideDirective(p *parser, data []byte) {
	p.defOpt = "override"
	defineDirective := map[string]directiveFunc{
		"define": defineDirective,
	}
	glog.V(1).Infof("override define? %q", data)
	if p.handleDirective(data, defineDirective) {
		return
	}
	// e.g. overrider foo := bar
	// line will be "foo := bar".
	if p.handleAssign(data) {
		return
	}
	p.defOpt = ""
	var line []byte
	line = append(line, []byte("override ")...)
	line = append(line, data...)
	p.handleRuleOrAssign(line)
	// TODO(ukai): evaluate now to detect invalid "override" directive here?
}

func handleExport(p *parser, data []byte, export bool) (hasEqual bool) {
	i := bytes.IndexByte(data, '=')
	if i > 0 {
		hasEqual = true
		switch data[i-1] {
		case ':', '+', '?':
			i--
		}
		data = data[:i]
	}
	east := &exportAST{
		expr:     data,
		hasEqual: hasEqual,
		export:   export,
	}
	east.srcpos = p.srcpos()
	glog.V(1).Infof("export %v", east)
	p.addStatement(east)
	return hasEqual
}

func exportDirective(p *parser, data []byte) {
	p.defOpt = "export"
	defineDirective := map[string]directiveFunc{
		"define": defineDirective,
	}
	glog.V(1).Infof("export define? %q", data)
	if p.handleDirective(data, defineDirective) {
		return
	}

	if !handleExport(p, data, true) {
		return
	}

	// e.g. export foo := bar
	// line will be "foo := bar".
	p.handleAssign(data)
}

func unexportDirective(p *parser, data []byte) {
	handleExport(p, data, false)
	return
}

func vpathDirective(p *parser, data []byte) {
	p.parseVpath(data)
}

func (p *parser) parse() (mk makefile, err error) {
	for !p.done {
		line := p.readLine()
		if glog.V(1) {
			glog.Infof("%s: %q", p.srcpos(), line)
		}
		if p.defineVar != nil {
			p.processDefine(line)
			if p.err != nil {
				return makefile{}, p.err
			}
			continue
		}
		p.defOpt = ""
		if p.inRecipe {
			if len(line) > 0 && line[0] == '\t' {
				cast := &commandAST{cmd: string(line[1:])}
				cast.srcpos = p.srcpos()
				p.addStatement(cast)
				continue
			}
		}
		p.parseLine(line)
		if p.err != nil {
			return makefile{}, p.err
		}
	}
	return p.mk, p.err
}

func (p *parser) parseLine(line []byte) {
	cline := concatline(line)
	if len(cline) == 0 {
		return
	}
	if glog.V(1) {
		glog.Infof("concatline:%q", cline)
	}
	var dline []byte
	cline, _ = removeComment(cline)
	dline = append(dline, cline...)
	dline = trimSpaceBytes(dline)
	if len(dline) == 0 {
		return
	}
	if glog.V(1) {
		glog.Infof("directive?: %q", dline)
	}
	if p.handleDirective(dline, makeDirectives) {
		return
	}
	if glog.V(1) {
		glog.Infof("rule or assign?: %q", line)
	}
	p.handleRuleOrAssign(line)
}

func (p *parser) processDefine(line []byte) {
	line = append(line, '\n')
	line = concatline(line)
	if line[len(line)-1] != '\n' {
		line = append(line, '\n')
	}
	if glog.V(1) {
		glog.Infof("concatline:%q", line)
	}
	if !p.isEndef(line) {
		p.inDef = append(p.inDef, line...)
		if p.inDef == nil {
			p.inDef = []byte{}
		}
		return
	}
	if p.inDef[len(p.inDef)-1] == '\n' {
		p.inDef = p.inDef[:len(p.inDef)-1]
	}
	glog.V(1).Infof("multilineAssign %q %q", p.defineVar, p.inDef)
	aast, err := newAssignAST(p, p.defineVar, p.inDef, "=")
	if err != nil {
		p.err = p.srcpos().errorf("assign error %q=%q: %v", p.defineVar, p.inDef, err)
		return
	}
	aast.srcpos = p.srcpos()
	aast.srcpos.lineno -= bytes.Count(p.inDef, []byte{'\n'})
	p.addStatement(aast)
	p.defineVar = nil
	p.inDef = nil
	return
}

func (p *parser) isEndef(line []byte) bool {
	if bytes.Equal(line, []byte("endef")) {
		return true
	}
	w, data := firstWord(line)
	if bytes.Equal(w, []byte("endef")) {
		data, _ = removeComment(data)
		data = trimLeftSpaceBytes(data)
		if len(data) > 0 {
			warnNoPrefix(p.srcpos(), `extraneous text after "endef" directive`)
		}
		return true
	}
	return false
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
	glog.Infof("parse Makefile %q", filename)
	mk, hash, ok, err := makefileCache.lookup(filename)
	if ok {
		if glog.V(1) {
			glog.Infof("makefile cache hit for %q", filename)
		}
		return mk, hash, err
	}
	if glog.V(1) {
		glog.Infof("reading makefile %q", filename)
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
