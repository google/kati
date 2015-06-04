package main

import (
	"bytes"
	"fmt"
	"strings"
)

const BootstrapMakefile = "*bootstrap*"

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
	lhs Value
	rhs Value
	op  string
	opt string // "override", "export"
}

func (ast *AssignAST) eval(ev *Evaluator) {
	ev.evalAssign(ast)
}

func (ast *AssignAST) evalRHS(ev *Evaluator, lhs string) Var {
	origin := "file"
	if ast.filename == BootstrapMakefile {
		origin = "default"
	}
	if ast.opt == "override" {
		origin = "override"
	}
	// TODO(ukai): handle ast.opt == "export"
	switch ast.op {
	case ":=":
		var buf bytes.Buffer
		ast.rhs.Eval(&buf, ev)
		return SimpleVar{value: buf.Bytes(), origin: origin}
	case "=":
		return RecursiveVar{expr: ast.rhs, origin: origin}
	case "+=":
		prev := ev.LookupVarInCurrentScope(lhs)
		if !prev.IsDefined() {
			return RecursiveVar{expr: ast.rhs, origin: origin}
		}
		return prev.AppendVar(ev, ast.rhs)
	case "?=":
		prev := ev.LookupVarInCurrentScope(lhs)
		if prev.IsDefined() {
			return prev
		}
		return RecursiveVar{expr: ast.rhs, origin: origin}
	default:
		panic(fmt.Sprintf("unknown assign op: %q", ast.op))
	}
}

func (ast *AssignAST) show() {
	Log("%s %s %s %q", ast.opt, ast.lhs, ast.op, ast.rhs)
}

// Note we cannot be sure what this is, until all variables in |expr|
// are expanded.
type MaybeRuleAST struct {
	ASTBase
	expr      Value
	term      byte // Either ':', '=', or 0
	afterTerm []byte
}

func (ast *MaybeRuleAST) eval(ev *Evaluator) {
	ev.evalMaybeRule(ast)
}

func (ast *MaybeRuleAST) show() {
	Log("%s", ast.expr)
}

type CommandAST struct {
	ASTBase
	cmd string
}

func (ast *CommandAST) eval(ev *Evaluator) {
	ev.evalCommand(ast)
}

func (ast *CommandAST) show() {
	Log("\t%s", strings.Replace(ast.cmd, "\n", `\n`, -1))
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

type IfAST struct {
	ASTBase
	op         string
	lhs        string
	rhs        string // Empty if |op| is ifdef or ifndef.
	trueStmts  []AST
	falseStmts []AST
}

func (ast *IfAST) eval(ev *Evaluator) {
	ev.evalIf(ast)
}

func (ast *IfAST) show() {
	// TODO
	Log("if")
}

type ExportAST struct {
	ASTBase
	expr   []byte
	export bool
}

func (ast *ExportAST) eval(ev *Evaluator) {
	ev.evalExport(ast)
}

func (ast *ExportAST) show() {
	// TODO
	Log("export")
}
