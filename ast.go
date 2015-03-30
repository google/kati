package main

import "fmt"

type AST interface {
	eval(*Evaluator)
	show()
}

type ASTBase struct {
	filename string
	lineno   int
}

type AssignAST struct {
	ASTBase
	lhs string
	rhs string
	op  string
}

func (ast *AssignAST) eval(ev *Evaluator) {
	ev.evalAssign(ast)
}

func (ast *AssignAST) evalRHS(ev *Evaluator) string {
	switch ast.op {
	case ":=":
		return ev.evalExpr(ast.rhs)
	case "=":
		return ast.rhs
	default: // "+=", "?="
		panic(fmt.Sprintf("not implemented assign op: %q", ast.op))
	}
}

func (ast *AssignAST) show() {
	Log("%s=%s", ast.lhs, ast.rhs)
}

type RuleAST struct {
	ASTBase
	lhs  string
	rhs  string
	cmds []string
	cmdLineno int
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

type IncludeAST struct {
	ASTBase
	expr string
	op   string
}

func (ast *IncludeAST) eval(ev *Evaluator) {
	ev.evalInclude(ast)
}

func (ast *IncludeAST) show() {
	Log("include %s", ast.expr)
}
