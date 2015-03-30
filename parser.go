package main

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strings"
)

type Makefile struct {
	stmts []AST
}

type parser struct {
	rd     *bufio.Reader
	mk     Makefile
	lineno int
	done   bool
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

func (p *parser) parseAssign(lhs string) AST {
	ast := &AssignAST{lhs: lhs}
	ast.lineno = p.lineno
	ast.rhs = strings.TrimSpace(p.readUntilEol())
	return ast
}

func (p *parser) parseRule(lhs string) AST {
	ast := &RuleAST{lhs: lhs}
	ast.lineno = p.lineno
	ast.rhs = strings.TrimSpace(p.readUntilEol())
	for {
		ch, err := p.readByte()
		if err != nil {
			return ast
		}
		switch ch {
		case '\n':
			continue
		case '\t':
			ast.cmds = append(ast.cmds, strings.TrimSpace(p.readUntilEol()))
			continue
		default:
			p.unreadByte()
			return ast
		}
	}
}

func (p *parser) parse() (Makefile, error) {
	for {
		tok, err := p.getNextToken()
		Log("tok=%s", tok)
		if err == io.EOF {
			return p.mk, nil
		} else if err != nil {
			return p.mk, err
		}
		switch tok {
		default:
			ntok, err := p.getNextToken()
			if err != nil {
				return p.mk, err
			}
			switch ntok {
			case "=":
				ast := p.parseAssign(tok)
				p.mk.stmts = append(p.mk.stmts, ast)
			case ":":
				ast := p.parseRule(tok)
				p.mk.stmts = append(p.mk.stmts, ast)
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
	return Makefile{}, errors.New("No targets specified and no makefile found.")
}
