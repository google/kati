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
	// TODO(ukai): use Value.
	lhs string
	rhs string
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
		rexpr, _, err := parseExpr([]byte(ast.rhs), nil)
		if err != nil {
			panic(fmt.Errorf("parse assign rhs %s:%d %v", ast.filename, ast.lineno, err))
		}
		var buf bytes.Buffer
		rexpr.Eval(&buf, ev)
		return SimpleVar{value: buf.Bytes(), origin: origin}
	case "=":
		v, _, err := parseExpr([]byte(ast.rhs), nil)
		if err != nil {
			panic(err)
		}
		return RecursiveVar{expr: v, origin: origin}
	case "+=":
		prev := ev.LookupVarInCurrentScope(lhs)
		if !prev.IsDefined() {
			v, _, err := parseExpr([]byte(ast.rhs), nil)
			if err != nil {
				panic(err)
			}
			return RecursiveVar{expr: v, origin: origin}
		}
		return prev.Append(ev, ast.rhs)
	case "?=":
		prev := ev.LookupVarInCurrentScope(lhs)
		if prev.IsDefined() {
			return prev
		}
		v, _, err := parseExpr([]byte(ast.rhs), nil)
		if err != nil {
			panic(err)
		}
		return RecursiveVar{expr: v, origin: origin}
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
	expr           string
	equalIndex     int
	semicolonIndex int
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
	name   string
	export bool
}

func (ast *ExportAST) eval(ev *Evaluator) {
	ev.evalExport(ast)
}

func (ast *ExportAST) show() {
	// TODO
	Log("if")
}
