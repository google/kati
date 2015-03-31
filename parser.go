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
	Log("parseAssign %s %s", line, line[sep:esep])
	ast := &AssignAST{
		lhs: string(bytes.TrimSpace(line[:sep])),
		rhs: string(bytes.TrimLeft(line[esep:], " \t")),
		op:  string(line[sep:esep]),
	}
	ast.filename = p.filename
	ast.lineno = p.lineno
	return ast
}

func (p *parser) parseRule(line []byte, sep int) AST {
	lhs := string(bytes.TrimSpace(line[:sep]))
	rhs := string(bytes.TrimSpace(line[sep+1:]))
	ast := &RuleAST{
		lhs: lhs,
		rhs: rhs,
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

func (p *parser) parseEq(s string) (string, string, bool) {
	if len(s) == 0 || s[0] != '(' {
		return "", "", false
	}

	i := 0
	parenCnt := 0
	inRhs := false
	var lhs []byte
	var rhs []byte
	for {
		i++
		if i == len(s) {
			return "", "", false
		}
		ch := s[i]
		if ch == '(' {
			parenCnt++
		} else if ch == ')' {
			parenCnt--
			if parenCnt < 0 {
				if inRhs {
					break
				} else {
					return "", "", false
				}
			}
		} else if ch == ',' {
			if inRhs {
				return "", "", false
			} else {
				inRhs = true
				continue
			}
		}
		if inRhs {
			rhs = append(rhs, ch)
		} else {
			lhs = append(lhs, ch)
		}
	}
	return string(lhs), string(rhs), true
}

func (p *parser) parseIfeq(line string, oplen int) AST {
	lhs, rhs, ok := p.parseEq(strings.TrimSpace(line[oplen+1:]))
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

func (p *parser) parseLine(line string) AST {
	stripped := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(stripped, "include ") {
		return p.parseInclude(stripped, len("include"))
	}
	if strings.HasPrefix(stripped, "-include ") {
		return p.parseInclude(stripped, len("-include"))
	}
	if strings.HasPrefix(stripped, "ifdef ") {
		p.parseIfdef(stripped, len("ifdef"))
		return nil
	}
	if strings.HasPrefix(stripped, "ifndef ") {
		p.parseIfdef(stripped, len("ifndef"))
		return nil
	}
	if strings.HasPrefix(stripped, "ifeq ") {
		p.parseIfeq(stripped, len("ifeq"))
		return nil
	}
	if strings.HasPrefix(stripped, "ifneq ") {
		p.parseIfeq(stripped, len("ifneq"))
		return nil
	}
	if strings.HasPrefix(stripped, "else") {
		p.parseElse(stripped)
		return nil
	}
	if strings.HasPrefix(stripped, "endif") {
		p.parseEndif(stripped)
		return nil
	}
	ast := &RawExprAST{expr: line}
	ast.filename = p.filename
	ast.lineno = p.lineno
	return ast
}

func (p *parser) parse() (mk Makefile, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	for !p.done {
		line := p.readLine()

		var ast AST
		for i, ch := range line {
			switch ch {
			case ':':
				if i+1 < len(line) && line[i+1] == '=' {
					ast = p.parseAssign(line, i, i+2)
				} else {
					ast = p.parseRule(line, i)
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
		if ast == nil && len(bytes.TrimSpace(line)) > 0 {
			ast = p.parseLine(string(line))
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
