package main

const (
	AST_ASSIGN = iota
	AST_RULE
)

type AST interface {
	typ() int
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

func (ast *AssignAST) typ() int {
	return AST_ASSIGN
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

func (ast *RuleAST) typ() int {
	return AST_RULE
}

func (ast *RuleAST) show() {
	Log("%s: %s", ast.lhs, ast.rhs)
	for _, cmd := range ast.cmds {
		Log("\t%s", cmd)
	}
}
