package main

import (
	"bufio"
	"bytes"
	"errors"
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

func isdigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isident(ch byte) bool {
	return (ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch == '_' || ch == '.')
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

func (p *parser) readByte() (byte, error) {
	ch, err := p.rd.ReadByte()
	if err != nil {
		p.done = true
	}
	return ch, err
}

func (p *parser) unreadByte() {
	p.rd.UnreadByte()
}

func (p *parser) skipWhiteSpaces() error {
	for {
		ch, err := p.readByte()
		if err != nil {
			return err
		}
		switch ch {
		case '\n':
			p.lineno++
			fallthrough
		case '\r', ' ':
			continue
		default:
			p.unreadByte()
			return nil
		}
	}
}

func (p *parser) getNextToken() (string, error) {
	if err := p.skipWhiteSpaces(); err != nil {
		return "", err
	}
	ch, err := p.readByte()
	if err != nil {
		return "", errors.New("TODO")
	}
	switch ch {
	case '$', '=':
		return string(ch), nil
	case ':':
		var s []byte
		s = append(s, ch)
		ch, err := p.readByte()
		if ch == ':' {
			ch, err = p.readByte()
		}
		if err != nil {
			return string(s), err
		}
		if ch == '=' {
			s = append(s, ch)
		} else {
			p.unreadByte()
		}
		return string(s), nil
	default:
		if isident(ch) {
			var s []byte
			s = append(s, ch)
			for {
				ch, err := p.readByte()
				if err != nil {
					return string(s), err
				}
				if isident(ch) || isdigit(ch) {
					s = append(s, ch)
				} else {
					p.unreadByte()
					return string(s), nil
				}
			}
		}
	}

	return "", errors.New("foobar")
}

func (p *parser) readUntilEol() string {
	var r []byte
	for {
		ch, err := p.readByte()
		if err != nil || ch == '\n' {
			return string(r)
		}
		r = append(r, ch)
	}
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

func (p *parser) parse() (Makefile, error) {
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
