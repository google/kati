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
	filename string
	stmts    []AST
}

type ifState struct {
	ast     *IfAST
	inElse  bool
	numNest int
}

type parser struct {
	rd        *bufio.Reader
	mk        Makefile
	lineno    int
	elineno   int // lineno == elineno unless there is trailing '\'.
	unBuf     []byte
	hasUnBuf  bool
	done      bool
	outStmts  *[]AST
	ifStack   []ifState
	inDef     []string
	numIfNest int
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

	p.lineno = p.elineno
	line, err := p.rd.ReadBytes('\n')
	p.lineno++
	p.elineno = p.lineno
	if err == io.EOF {
		p.done = true
	} else if err != nil {
		panic(err)
	}

	line = bytes.TrimRight(line, "\n")

	return line
}

func removeComment(line []byte) []byte {
	var parenStack []byte
	for i, ch := range line {
		switch ch {
		case '(', '{':
			parenStack = append(parenStack, ch)
		case ')', '}':
			if len(parenStack) > 0 {
				parenStack = parenStack[:len(parenStack)-1]
			}
		case '#':
			if len(parenStack) == 0 {
				return line[:i]
			}
		}
	}
	return line
}

func (p *parser) processMakefileLine(line []byte) []byte {
	// TODO: Handle \\ at the end of the line?
	for len(line) > 0 && line[len(line)-1] == '\\' {
		line = line[:len(line)-1]
		lineno := p.lineno
		nline := p.readLine()
		p.lineno = lineno
		line = append(line, nline...)
	}
	return removeComment(line)
}

func (p *parser) processRecipeLine(line []byte) []byte {
	// TODO: Handle \\ at the end of the line?
	for len(line) > 0 && line[len(line)-1] == '\\' {
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
	ast := &AssignAST{
		lhs: string(bytes.TrimSpace(line[:sep])),
		rhs: string(bytes.TrimLeft(line[esep:], " \t")),
		op:  string(line[sep:esep]),
	}
	ast.filename = p.mk.filename
	ast.lineno = p.lineno
	return ast
}

func (p *parser) parseMaybeRule(line string, semicolonIndex int) AST {
	if len(strings.TrimSpace(line)) == 0 {
		return nil
	}

	ast := &MaybeRuleAST{}
	if i := semicolonIndex; i >= 0 {
		ast.expr = line[:i]
		ast.cmd = strings.TrimSpace(line[i+1:])
	} else {
		ast.expr = line
	}
	ast.filename = p.mk.filename
	ast.lineno = p.lineno
	/*
		ast.cmdLineno = p.elineno + 1
		for {
			line := p.readRecipeLine()
			if len(line) == 0 {
				break
			} else if line[0] == '\t' {
				ast.cmds = append(ast.cmds, string(bytes.TrimLeft(line, " \t")))
			} else {
				p.unreadLine(line)
				break
			}
		}
	*/
	return ast
}

func (p *parser) parseInclude(line string, oplen int) AST {
	ast := &IncludeAST{
		expr: line[oplen+1:],
		op:   line[:oplen],
	}
	ast.filename = p.mk.filename
	ast.lineno = p.lineno
	return ast
}

func (p *parser) parseIfdef(line string, oplen int) AST {
	ast := &IfAST{
		op:  line[:oplen],
		lhs: strings.TrimSpace(line[oplen+1:]),
	}
	ast.filename = p.mk.filename
	ast.lineno = p.lineno
	p.addStatement(ast)
	p.ifStack = append(p.ifStack, ifState{ast: ast, numNest: p.numIfNest})
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

func parseTwoQuotes(s string) ([]string, bool) {
	toks := splitSpaces(s)
	if len(toks) != 2 {
		return nil, false
	}
	var args []string
	for _, tok := range toks {
		if len(tok) < 2 {
			return nil, false
		}
		ti := len(tok) - 1
		if tok[0] != tok[ti] || (tok[0] != '\'' && tok[ti] != '"') {
			return nil, false
		}
		args = append(args, tok[1:ti])
	}
	return args, true
}

func parseEq(s string) (string, string, bool) {
	args, _, err := parseExpr(s)
	if err != nil {
		args, ok := parseTwoQuotes(s)
		if ok {
			return args[0], args[1], true
		}
		return "", "", false
	}
	if len(args) != 2 {
		return "", "", false
	}
	// TODO: check rest?
	return args[0], strings.TrimLeft(args[1], " \t"), true
}

func (p *parser) parseIfeq(line string, oplen int) AST {
	lhs, rhs, ok := parseEq(strings.TrimSpace(line[oplen+1:]))
	if !ok {
		Error(p.mk.filename, p.lineno, `*** invalid syntax in conditional.`)
	}

	ast := &IfAST{
		op:  line[:oplen],
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

func (p *parser) parseElse(line string) {
	p.checkIfStack("else")
	state := &p.ifStack[len(p.ifStack)-1]
	if state.inElse {
		Error(p.mk.filename, p.lineno, `*** only one "else" per conditional.`)
	}
	state.inElse = true
	p.outStmts = &state.ast.falseStmts

	nextIf := strings.TrimSpace(line[len("else"):])
	if len(nextIf) == 0 {
		return
	}
	var ifDirectives = map[string]func(*parser, string){
		"ifdef ":  ifdefDirective,
		"ifndef ": ifndefDirective,
		"ifeq ":   ifeqDirective,
		"ifneq ":  ifneqDirective,
	}
	p.numIfNest = state.numNest + 1
	if p.parseKeywords(nextIf, ifDirectives) {
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

var makeDirectives = map[string]func(*parser, string){
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

func (p *parser) parseKeywords(line string, directives map[string]func(*parser, string)) bool {
	stripped := strings.TrimLeft(line, " \t")
	for prefix, f := range directives {
		if strings.HasPrefix(stripped, prefix) {
			f(p, stripped)
			return true
		}
	}
	return false
}

func (p *parser) isDirective(line string, directives map[string]func(*parser, string)) bool {
	stripped := strings.TrimLeft(line, " \t")
	for prefix, _ := range directives {
		if strings.HasPrefix(stripped, prefix) {
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
			line = p.processMakefileLine(line)
			if strings.TrimLeft(string(line), " ") == "endef" {
				Log("multilineAssign %q", p.inDef)
				ast := &AssignAST{
					lhs: p.inDef[0],
					rhs: strings.Join(p.inDef[1:], "\n"),
					op:  "=",
				}
				ast.filename = p.mk.filename
				ast.lineno = p.lineno - len(p.inDef)
				p.addStatement(ast)
				p.inDef = nil
				continue
			}
			p.inDef = append(p.inDef, string(line))
			continue
		}

		if len(line) == 0 {
			continue
		}

		if p.isDirective(string(line), makeDirectives) {
			line = p.processMakefileLine(line)
			p.parseKeywords(string(line), makeDirectives)
			continue
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
					ast = p.parseAssign(line, i, i+2)
				} else {
					isRule = true
				}
			case ';':
				semicolonIndex = i
			case '=':
				if !isRule {
					ast = p.parseAssign(line, i, i+1)
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
			ast = p.parseMaybeRule(string(line), semicolonIndex)
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

func ParseMakefileString(s string, name string, lineno int) (Makefile, error) {
	rd := strings.NewReader(s)
	parser := newParser(rd, name)
	parser.lineno = lineno - 1
	parser.elineno = lineno
	return parser.parse()
}
