// Copyright 2015 Google Inc. All rights reserved
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kati

import (
	"bytes"
	"fmt"
	"strings"
)

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
	if ast.filename == bootstrapMakefileName {
		origin = "default"
	}
	if ast.opt == "override" {
		origin = "override"
	}
	// TODO(ukai): handle ast.opt == "export"
	switch ast.op {
	case ":=":
		switch v := ast.rhs.(type) {
		case literal:
			return &SimpleVar{value: v.String(), origin: origin}
		case tmpval:
			return &SimpleVar{value: v.String(), origin: origin}
		default:
			var buf bytes.Buffer
			v.Eval(&buf, ev)
			return &SimpleVar{value: buf.String(), origin: origin}
		}
	case "=":
		return &RecursiveVar{expr: ast.rhs, origin: origin}
	case "+=":
		prev := ev.LookupVarInCurrentScope(lhs)
		if !prev.IsDefined() {
			return &RecursiveVar{expr: ast.rhs, origin: origin}
		}
		return prev.AppendVar(ev, ast.rhs)
	case "?=":
		prev := ev.LookupVarInCurrentScope(lhs)
		if prev.IsDefined() {
			return prev
		}
		return &RecursiveVar{expr: ast.rhs, origin: origin}
	default:
		panic(fmt.Sprintf("unknown assign op: %q", ast.op))
	}
}

func (ast *AssignAST) show() {
	Logf("%s %s %s %q", ast.opt, ast.lhs, ast.op, ast.rhs)
}

// MaybeRuleAST is an ast for rule line.
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
	Logf("%s", ast.expr)
}

type CommandAST struct {
	ASTBase
	cmd string
}

func (ast *CommandAST) eval(ev *Evaluator) {
	ev.evalCommand(ast)
}

func (ast *CommandAST) show() {
	Logf("\t%s", strings.Replace(ast.cmd, "\n", `\n`, -1))
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
	Logf("include %s", ast.expr)
}

type IfAST struct {
	ASTBase
	op         string
	lhs        Value
	rhs        Value // Empty if |op| is ifdef or ifndef.
	trueStmts  []AST
	falseStmts []AST
}

func (ast *IfAST) eval(ev *Evaluator) {
	ev.evalIf(ast)
}

func (ast *IfAST) show() {
	// TODO
	Logf("if")
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
	Logf("export")
}
