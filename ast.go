package main

type AST interface {
	eval(*Evaluator)
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

func (ast *AssignAST) eval(ev *Evaluator) {
	ev.evalAssign(ast)
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

func (ast *RuleAST) eval(ev *Evaluator) {
	ev.evalRule(ast)
}

func (ast *RuleAST) show() {
	Log("%s: %s", ast.lhs, ast.rhs)
	for _, cmd := range ast.cmds {
		Log("\t%s", cmd)
	}
}
