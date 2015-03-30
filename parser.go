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

type parser struct {
	rd       *bufio.Reader
	mk       Makefile
	filename string
	lineno   int
	elineno  int // lineno == elineno unless there is trailing '\'.
	unBuf    []byte
	hasUnBuf bool
	done     bool
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
	return &parser{
		rd:       bufio.NewReader(rd),
		filename: filename,
	}
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

func (p *parser) parseLine(line string) AST {
	if strings.HasPrefix(line, "include ") {
		return p.parseInclude(line, len("include"))
	}
	if strings.HasPrefix(line, "-include ") {
		return p.parseInclude(line, len("-include"))
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
			case '?':
				panic("TODO")
			}
			if ast != nil {
				p.mk.stmts = append(p.mk.stmts, ast)
				break
			}
		}
		if ast == nil && len(bytes.TrimSpace(line)) > 0 {
			ast = p.parseLine(string(line))
			p.mk.stmts = append(p.mk.stmts, ast)
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
