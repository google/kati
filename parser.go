package main

//go:generate go run testcase/gen_testcase_parse_benchmark.go
//
// $ go generate
// $ go test -bench .

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
)

type Makefile struct {
	filename string
	stmts    []AST
}

type ifState struct {
	ast     *IfAST
	inElse  bool
	numNest int
}

type parser struct {
	rd          *bufio.Reader
	mk          Makefile
	lineno      int
	elineno     int // lineno == elineno unless there is trailing '\'.
	linenoFixed bool
	unBuf       []byte
	hasUnBuf    bool
	done        bool
	outStmts    *[]AST
	ifStack     []ifState
	inDef       []string
	defOpt      string
	numIfNest   int
}

func newParser(rd io.Reader, filename string) *parser {
	p := &parser{
		rd: bufio.NewReader(rd),
	}
	p.mk.filename = filename
	p.outStmts = &p.mk.stmts
	return p
}

func (p *parser) addStatement(ast AST) {
	*p.outStmts = append(*p.outStmts, ast)
}

func (p *parser) readLine() []byte {
	if p.hasUnBuf {
		p.hasUnBuf = false
		return p.unBuf
	}

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
		panic(fmt.Errorf("readline %s:%d: %v", p.mk.filename, p.lineno, err))
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
				parenStack = parenStack[:len(parenStack)-1]
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

func (p *parser) unreadLine(line []byte) {
	if p.hasUnBuf {
		panic("unreadLine twice!")
	}
	p.unBuf = line
	p.hasUnBuf = true
}

func (p *parser) parseAssign(line []byte, sep, esep int) AST {
	Log("parseAssign %q op:%q", line, line[sep:esep])
	// TODO(ukai): parse expr here.
	ast := &AssignAST{
		lhs: string(bytes.TrimSpace(line[:sep])),
		rhs: trimLeftSpace(string(line[esep:])),
		op:  string(line[sep:esep]),
		opt: p.defOpt,
	}
	ast.filename = p.mk.filename
	ast.lineno = p.lineno
	return ast
}

func (p *parser) parseMaybeRule(line string, equalIndex, semicolonIndex int) AST {
	if len(strings.TrimSpace(line)) == 0 {
		return nil
	}

	// Either '=' or ';' is used.
	if equalIndex >= 0 && semicolonIndex >= 0 {
		if equalIndex < semicolonIndex {
			semicolonIndex = -1
		} else {
			equalIndex = -1
		}
	}

	ast := &MaybeRuleAST{
		expr:           line,
		equalIndex:     equalIndex,
		semicolonIndex: semicolonIndex,
	}
	ast.filename = p.mk.filename
	ast.lineno = p.lineno
	return ast
}

func (p *parser) parseInclude(line string, oplen int) AST {
	// TODO(ukai): parse expr here
	ast := &IncludeAST{
		expr: line[oplen+1:],
		op:   line[:oplen],
	}
	ast.filename = p.mk.filename
	ast.lineno = p.lineno
	return ast
}

func (p *parser) parseIfdef(line string, oplen int) AST {
	// TODO(ukai): parse expr here.
	ast := &IfAST{
		op:  line[:oplen],
		lhs: line[oplen+1:],
	}
	ast.filename = p.mk.filename
	ast.lineno = p.lineno
	p.addStatement(ast)
	p.ifStack = append(p.ifStack, ifState{ast: ast, numNest: p.numIfNest})
	p.outStmts = &ast.trueStmts
	return ast
}

func (p *parser) parseTwoQuotes(s string, op string) ([]string, bool) {
	var args []string
	for i := 0; i < 2; i++ {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, false
		}
		quote := s[0]
		if quote != '\'' && quote != '"' {
			return nil, false
		}
		end := strings.IndexByte(s[1:], quote) + 1
		if end < 0 {
			return nil, false
		}
		args = append(args, s[1:end])
		s = s[end+1:]
	}
	if len(s) > 0 {
		Error(p.mk.filename, p.lineno, `extraneous text after %q directive`, op)
	}
	return args, true
}

// parse
//  "(lhs, rhs)"
//  "lhs, rhs"
func (p *parser) parseEq(s string, op string) (string, string, bool) {
	if s[0] == '(' && s[len(s)-1] == ')' {
		s = s[1 : len(s)-1]
		term := []byte{','}
		in := []byte(s)
		v, n, err := parseExpr(in, term)
		if err != nil {
			return "", "", false
		}
		lhs := v.String()
		n++
		n += skipSpaces(in[n:], nil)
		v, n, err = parseExpr(in[n:], nil)
		if err != nil {
			return "", "", false
		}
		rhs := v.String()
		return lhs, rhs, true
	}
	args, ok := p.parseTwoQuotes(s, op)
	if !ok {
		return "", "", false
	}
	return args[0], args[1], true
}

func (p *parser) parseIfeq(line string, oplen int) AST {
	op := line[:oplen]
	lhs, rhs, ok := p.parseEq(strings.TrimSpace(line[oplen+1:]), op)
	if !ok {
		Error(p.mk.filename, p.lineno, `*** invalid syntax in conditional.`)
	}

	ast := &IfAST{
		op:  op,
		lhs: lhs,
		rhs: rhs,
	}
	ast.filename = p.mk.filename
	ast.lineno = p.lineno
	p.addStatement(ast)
	p.ifStack = append(p.ifStack, ifState{ast: ast, numNest: p.numIfNest})
	p.outStmts = &ast.trueStmts
	return ast
}

func (p *parser) checkIfStack(curKeyword string) {
	if len(p.ifStack) == 0 {
		Error(p.mk.filename, p.lineno, `*** extraneous %q.`, curKeyword)
	}
}

func (p *parser) parseElse(line []byte) {
	p.checkIfStack("else")
	state := &p.ifStack[len(p.ifStack)-1]
	if state.inElse {
		Error(p.mk.filename, p.lineno, `*** only one "else" per conditional.`)
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
	WarnNoPrefix(p.mk.filename, p.lineno, "extraneous text after `else` directive")
}

func (p *parser) parseEndif(line string) {
	p.checkIfStack("endif")
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
}

// TODO(ukai): use []byte
func (p *parser) isDirective(line []byte, directives map[string]directiveFunc) (directiveFunc, bool) {
	stripped := trimLeftSpaceBytes(line)
	// Fast paths.
	// TODO: Consider using a trie.
	if len(stripped) == 0 {
		return nil, false
	}
	if ch := stripped[0]; ch != 'i' && ch != '-' && ch != 's' && ch != 'e' && ch != 'd' && ch != 'o' {
		return nil, false
	}

	for prefix, f := range directives {
		if bytes.HasPrefix(stripped, []byte(prefix)) {
			return f, true
		}
		if prefix[len(prefix)-1] == ' ' && bytes.HasPrefix(stripped, []byte(prefix[:len(prefix)-1])) && stripped[len(prefix)-1] == '\t' {
			return f, true
		}
	}
	return nil, false
}

func includeDirective(p *parser, line []byte) []byte {
	p.addStatement(p.parseInclude(string(line), len("include")))
	return nil
}

func sincludeDirective(p *parser, line []byte) []byte {
	p.addStatement(p.parseInclude(string(line), len("-include")))
	return nil
}

func ifdefDirective(p *parser, line []byte) []byte {
	p.parseIfdef(string(line), len("ifdef"))
	return nil
}

func ifndefDirective(p *parser, line []byte) []byte {
	p.parseIfdef(string(line), len("ifndef"))
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
	// e.g. export foo := bar
	// line will be "foo := bar".
	return line
}

func (p *parser) parse() (mk Makefile, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in parse %s: %v", mk.filename, r)
		}
	}()
	for !p.done {
		line := p.readLine()

		if len(p.inDef) > 0 {
			line = p.processDefineLine(line)
			if trimLeftSpace(string(line)) == "endef" {
				Log("multilineAssign %q", p.inDef)
				ast := &AssignAST{
					lhs: p.inDef[0],
					rhs: strings.Join(p.inDef[1:], "\n"),
					op:  "=",
					opt: p.defOpt,
				}
				ast.filename = p.mk.filename
				ast.lineno = p.lineno - len(p.inDef)
				p.addStatement(ast)
				p.inDef = nil
				p.defOpt = ""
				continue
			}
			p.inDef = append(p.inDef, string(line))
			continue
		}
		p.defOpt = ""

		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		if f, ok := p.isDirective(line, makeDirectives); ok {
			line = p.processMakefileLine(trimLeftSpaceBytes(line))
			line = f(p, line)
			if len(line) == 0 {
				continue
			}
		}
		if line[0] == '\t' {
			ast := &CommandAST{cmd: string(p.processRecipeLine(line[1:]))}
			ast.filename = p.mk.filename
			ast.lineno = p.lineno
			p.addStatement(ast)
			continue
		}

		line = p.processMakefileLine(line)

		var ast AST
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
					Warn(p.mk.filename, p.lineno, "Unmatched parens: %s", line)
				} else {
					parenStack = parenStack[:len(parenStack)-1]
				}
			}
			if len(parenStack) > 0 {
				continue
			}

			switch ch {
			case ':':
				if i+1 < len(line) && line[i+1] == '=' {
					if !isRule {
						ast = p.parseAssign(line, i, i+2)
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
					ast = p.parseAssign(line, i, i+1)
				}
				if equalIndex < 0 {
					equalIndex = i
				}
			case '?', '+':
				if !isRule && i+1 < len(line) && line[i+1] == '=' {
					ast = p.parseAssign(line, i, i+2)
				}
			}
			if ast != nil {
				p.addStatement(ast)
				break
			}
		}
		if ast == nil {
			ast = p.parseMaybeRule(string(line), equalIndex, semicolonIndex)
			if ast != nil {
				p.addStatement(ast)
			}
		}
	}
	return p.mk, nil
}

func ParseMakefileFd(filename string, f *os.File) (Makefile, error) {
	parser := newParser(f, filename)
	return parser.parse()
}

/*
func ParseMakefile(filename string) (Makefile, error) {
	Log("ParseMakefile %q", filename)
	f, err := os.Open(filename)
	if err != nil {
		return Makefile{}, err
	}
	defer f.Close()
	return ParseMakefileFd(filename, f)
}

func ParseDefaultMakefile() (Makefile, string, error) {
	candidates := []string{"GNUmakefile", "makefile", "Makefile"}
	for _, filename := range candidates {
		if exists(filename) {
			mk, err := ParseMakefile(filename)
			return mk, filename, err
		}
	}
	return Makefile{}, "", errors.New("no targets specified and no makefile found.")
}
*/

func GetDefaultMakefile() string {
	candidates := []string{"GNUmakefile", "makefile", "Makefile"}
	for _, filename := range candidates {
		if exists(filename) {
			return filename
		}
	}
	ErrorNoLocation("no targets specified and no makefile found.")
	panic("") // Cannot be reached.
}

func parseMakefileReader(rd io.Reader, name string, lineno int) (Makefile, error) {
	parser := newParser(rd, name)
	parser.lineno = lineno
	parser.elineno = lineno
	parser.linenoFixed = true
	return parser.parse()
}

func ParseMakefileString(s string, name string, lineno int) (Makefile, error) {
	return parseMakefileReader(strings.NewReader(s), name, lineno)
}

func ParseMakefileBytes(s []byte, name string, lineno int) (Makefile, error) {
	return parseMakefileReader(bytes.NewReader(s), name, lineno)
}

func ParseMakefile(s []byte, filename string) (Makefile, error) {
	Log("ParseMakefile %q", filename)
	parser := newParser(bytes.NewReader(s), filename)
	return parser.parse()
}
