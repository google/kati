package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

type Makefile struct {
	stmts []AST
}

type ifState struct {
	ast    *IfAST
	inElse bool
}

type parser struct {
	rd       *bufio.Reader
	mk       Makefile
	filename string
	lineno   int
	elineno  int // lineno == elineno unless there is trailing '\'.
	unBuf    []byte
	hasUnBuf bool
	done     bool
	outStmts *[]AST
	ifStack  []ifState
	inDef    []string
}

func exists(filename string) bool {
	f, err := os.Open(filename)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

func newParser(rd io.Reader, filename string) *parser {
	p := &parser{
		rd:       bufio.NewReader(rd),
		filename: filename,
	}
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

	p.lineno = p.elineno
	line, err := p.rd.ReadBytes('\n')
	p.lineno++
	p.elineno = p.lineno
	if err == io.EOF {
		p.done = true
	} else if err != nil {
		panic(err)
	}

	if len(line) > 0 {
		line = line[0 : len(line)-1]
	}

	// TODO: Handle \\ at the end of the line?
	for len(line) > 0 && line[len(line)-1] == '\\' {
		line = line[:len(line)-1]
		nline := p.readLine()
		p.elineno++
		line = append(line, nline...)
	}

	index := bytes.IndexByte(line, '#')
	if index >= 0 {
		line = line[:index]
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
	ast := &AssignAST{
		lhs: string(bytes.TrimSpace(line[:sep])),
		rhs: string(bytes.TrimLeft(line[esep:], " \t")),
		op:  string(line[sep:esep]),
	}
	ast.filename = p.filename
	ast.lineno = p.lineno
	return ast
}

func (p *parser) parseMaybeRule(line string) AST {
	if len(strings.TrimSpace(line)) == 0 {
		return nil
	}
	if line[0] == '\t' {
		Error(p.filename, p.lineno, "*** commands commence before first target.")
	}

	ast := &MaybeRuleAST{
		expr: line,
	}
	ast.filename = p.filename
	ast.lineno = p.lineno
	ast.cmdLineno = p.elineno + 1
	for {
		line := p.readLine()
		if len(line) == 0 {
			break
		} else if line[0] == '\t' {
			ast.cmds = append(ast.cmds, string(bytes.TrimSpace(line)))
		} else {
			p.unreadLine(line)
			break
		}
	}
	return ast
}

func (p *parser) parseInclude(line string, oplen int) AST {
	ast := &IncludeAST{
		expr: line[oplen+1:],
		op:   line[:oplen],
	}
	ast.filename = p.filename
	ast.lineno = p.lineno
	return ast
}

func (p *parser) parseIfdef(line string, oplen int) AST {
	ast := &IfAST{
		op:  line[:oplen],
		lhs: strings.TrimSpace(line[oplen+1:]),
	}
	ast.filename = p.filename
	ast.lineno = p.lineno
	p.addStatement(ast)
	p.ifStack = append(p.ifStack, ifState{ast: ast})
	p.outStmts = &ast.trueStmts
	return ast
}

func closeParen(ch byte) (byte, error) {
	switch ch {
	case '(':
		return ')', nil
	case '{':
		return '}', nil
	default:
		return 0, fmt.Errorf("unexpected paren %c", ch)
	}
}

// parseExpr parses s as expr.
// The expr should starts with '(' or '{' and returns strings
// separeted by ',' before ')' or '}' respectively, and an index for the rest.
func parseExpr(s string) ([]string, int, error) {
	if len(s) == 0 {
		return nil, 0, errors.New("empty expr")
	}
	paren, err := closeParen(s[0])
	if err != nil {
		return nil, 0, err
	}
	parenCnt := make(map[byte]int)
	i := 0
	ia := 1
	var args []string
Loop:
	for {
		i++
		if i == len(s) {
			return nil, 0, errors.New("unexpected end of expr")
		}
		ch := s[i]
		switch ch {
		case '(', '{':
			cch, err := closeParen(ch)
			if err != nil {
				return nil, 0, err
			}
			parenCnt[cch]++
		case ')', '}':
			parenCnt[ch]--
			if ch == paren && parenCnt[ch] < 0 {
				break Loop
			}
		case ',':
			if parenCnt[')'] == 0 && parenCnt['}'] == 0 {
				args = append(args, s[ia:i])
				ia = i + 1
			}
		}
	}
	args = append(args, s[ia:i])
	return args, i + 1, nil
}

func parseEq(s string) (string, string, bool) {
	args, _, err := parseExpr(s)
	if err != nil {
		return "", "", false
	}
	if len(args) != 2 {
		return "", "", false
	}
	// TODO: check rest?
	return args[0], args[1], true
}

func (p *parser) parseIfeq(line string, oplen int) AST {
	lhs, rhs, ok := parseEq(strings.TrimSpace(line[oplen+1:]))
	if !ok {
		Error(p.filename, p.lineno, `*** invalid syntax in conditional.`)
	}

	ast := &IfAST{
		op:  line[:oplen],
		lhs: lhs,
		rhs: rhs,
	}
	ast.filename = p.filename
	ast.lineno = p.lineno
	p.addStatement(ast)
	p.ifStack = append(p.ifStack, ifState{ast: ast})
	p.outStmts = &ast.trueStmts
	return ast
}

func (p *parser) checkIfStack(curKeyword string) {
	if len(p.ifStack) == 0 {
		Error(p.filename, p.lineno, `*** extraneous %q.`, curKeyword)
	}
}

func (p *parser) parseElse(line string) {
	p.checkIfStack("else")
	state := &p.ifStack[len(p.ifStack)-1]
	if state.inElse {
		Error(p.filename, p.lineno, `*** only one "else" per conditional.`)
	}
	state.inElse = true
	p.outStmts = &state.ast.falseStmts
}

func (p *parser) parseEndif(line string) {
	p.checkIfStack("endif")
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

var directives = map[string]func(*parser, string){
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
}

func (p *parser) parseKeywords(line string) bool {
	stripped := strings.TrimLeft(line, " \t")
	for prefix, f := range directives {
		if strings.HasPrefix(stripped, prefix) {
			f(p, stripped)
			return true
		}
	}
	return false
}

func includeDirective(p *parser, line string) {
	p.addStatement(p.parseInclude(line, len("include")))
}

func sincludeDirective(p *parser, line string) {
	p.addStatement(p.parseInclude(line, len("-include")))
}

func ifdefDirective(p *parser, line string) {
	p.parseIfdef(line, len("ifdef"))
}

func ifndefDirective(p *parser, line string) {
	p.parseIfdef(line, len("ifndef"))
}

func ifeqDirective(p *parser, line string) {
	p.parseIfeq(line, len("ifeq"))
}

func ifneqDirective(p *parser, line string) {
	p.parseIfeq(line, len("ifneq"))
}

func elseDirective(p *parser, line string) {
	p.parseElse(line)
}

func endifDirective(p *parser, line string) {
	p.parseEndif(line)
}

func defineDirective(p *parser, line string) {
	p.inDef = []string{strings.TrimLeft(line[len("define "):], " \t")}
}

func (p *parser) parse() (mk Makefile, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	for !p.done {
		line := p.readLine()

		if len(p.inDef) > 0 {
			if strings.TrimLeft(string(line), " ") == "endef" {
				Log("multilineAssign %q", p.inDef)
				ast := &AssignAST{
					lhs: p.inDef[0],
					rhs: strings.Join(p.inDef[1:], "\n"),
					op:  "=",
				}
				ast.filename = p.filename
				ast.lineno = p.lineno - len(p.inDef)
				p.addStatement(ast)
				p.inDef = nil
				continue
			}
			p.inDef = append(p.inDef, string(line))
			continue
		}

		if p.parseKeywords(string(line)) {
			continue
		}

		var ast AST
		for i, ch := range line {
			switch ch {
			case ':':
				if i+1 < len(line) && line[i+1] == '=' {
					ast = p.parseAssign(line, i, i+2)
				} else {
					ast = p.parseMaybeRule(string(line))
				}
			case '=':
				ast = p.parseAssign(line, i, i+1)
			case '?', '+':
				if i+1 < len(line) && line[i+1] == '=' {
					ast = p.parseAssign(line, i, i+2)
				}
			}
			if ast != nil {
				p.addStatement(ast)
				break
			}
		}
		if ast == nil {
			ast = p.parseMaybeRule(string(line))
			if ast != nil {
				p.addStatement(ast)
			}
		}
	}
	return p.mk, nil
}

func ParseMakefile(filename string) (Makefile, error) {
	f, err := os.Open(filename)
	if err != nil {
		return Makefile{}, err
	}
	defer f.Close()
	parser := newParser(f, filename)
	return parser.parse()
}

func ParseDefaultMakefile() (Makefile, error) {
	candidates := []string{"GNUmakefile", "makefile", "Makefile"}
	for _, filename := range candidates {
		if exists(filename) {
			return ParseMakefile(filename)
		}
	}
	return Makefile{}, errors.New("no targets specified and no makefile found.")
}
