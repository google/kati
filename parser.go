package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
)

type Makefile struct {
	stmts []AST
}

type parser struct {
	rd       *bufio.Reader
	mk       Makefile
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

func newParser(rd io.Reader) *parser {
	return &parser{
		rd: bufio.NewReader(rd),
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

func (p *parser) parseAssign(line []byte, sep int, typ int) AST {
	Log("parseAssign %s %d", line, sep)
	esep := sep + 1
	if typ != ASSIGN_RECURSIVE {
		esep++
	}
	lhs := string(bytes.TrimSpace(line[:sep]))
	rhs := string(bytes.TrimLeft(line[esep:], " \t"))
	ast := &AssignAST{lhs: lhs, rhs: rhs, assign_type: typ}
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
	ast.lineno = p.lineno
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
					ast = p.parseAssign(line, i, ASSIGN_SIMPLE)
				} else {
					ast = p.parseRule(line, i)
				}
			case '=':
				ast = p.parseAssign(line, i, ASSIGN_RECURSIVE)
			case '?':
				panic("TODO")
			}
			if ast != nil {
				p.mk.stmts = append(p.mk.stmts, ast)
				break
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
	parser := newParser(f)
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
