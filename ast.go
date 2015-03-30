package main

type ASTType int

const (
	ASTAssign ASTType = iota
	ASTRule
)

type AST interface {
	typ() ASTType
	show()
}

type ASTBase struct {
	lineno int
}

type AssignAST struct {
	ASTBase
	lhs string
	rhs string
}

func (ast *AssignAST) typ() ASTType {
	return ASTAssign
}

func (ast *AssignAST) show() {
	Log("%s=%s", ast.lhs, ast.rhs)
}

type RuleAST struct {
	ASTBase
	lhs  string
	rhs  string
	cmds []string
}

func (ast *RuleAST) typ() ASTType {
	return ASTRule
}

func (ast *RuleAST) show() {
	Log("%s: %s", ast.lhs, ast.rhs)
	for _, cmd := range ast.cmds {
		Log("\t%s", cmd)
	}
}
