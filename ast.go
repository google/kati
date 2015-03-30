package main

type AST interface {
	eval(*Evaluator)
	show()
}

type ASTBase struct {
	filename string
	lineno   int
}

const (
	ASSIGN_SIMPLE      = iota // :=
	ASSIGN_RECURSIVE          // =
	ASSIGN_APPEND             // +=
	ASSIGN_CONDITIONAL        // ?=
)

type AssignAST struct {
	ASTBase
	lhs         string
	rhs         string
	assign_type int
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

type RawExprAST struct {
	ASTBase
	expr string
}

func (ast *RawExprAST) eval(ev *Evaluator) {
	ev.evalRawExpr(ast)
}

func (ast *RawExprAST) show() {
	Log("%s", ast.expr)
}
