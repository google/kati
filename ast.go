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

type ast interface {
	eval(*Evaluator)
	show()
}

type astBase struct {
	filename string
	lineno   int
}

type assignAST struct {
	astBase
	lhs Value
	rhs Value
	op  string
	opt string // "override", "export"
}

func (ast *assignAST) eval(ev *Evaluator) {
	ev.evalAssign(ast)
}

func (ast *assignAST) evalRHS(ev *Evaluator, lhs string) Var {
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
			return &simpleVar{value: v.String(), origin: origin}
		case tmpval:
			return &simpleVar{value: v.String(), origin: origin}
		default:
			var buf bytes.Buffer
			v.Eval(&buf, ev)
			return &simpleVar{value: buf.String(), origin: origin}
		}
	case "=":
		return &recursiveVar{expr: ast.rhs, origin: origin}
	case "+=":
		prev := ev.lookupVarInCurrentScope(lhs)
		if !prev.IsDefined() {
			return &recursiveVar{expr: ast.rhs, origin: origin}
		}
		return prev.AppendVar(ev, ast.rhs)
	case "?=":
		prev := ev.lookupVarInCurrentScope(lhs)
		if prev.IsDefined() {
			return prev
		}
		return &recursiveVar{expr: ast.rhs, origin: origin}
	default:
		panic(fmt.Sprintf("unknown assign op: %q", ast.op))
	}
}

func (ast *assignAST) show() {
	logf("%s %s %s %q", ast.opt, ast.lhs, ast.op, ast.rhs)
}

// maybeRuleAST is an ast for rule line.
// Note we cannot be sure what this is, until all variables in |expr|
// are expanded.
type maybeRuleAST struct {
	astBase
	expr      Value
	term      byte // Either ':', '=', or 0
	afterTerm []byte
}

func (ast *maybeRuleAST) eval(ev *Evaluator) {
	ev.evalMaybeRule(ast)
}

func (ast *maybeRuleAST) show() {
	logf("%s", ast.expr)
}

type commandAST struct {
	astBase
	cmd string
}

func (ast *commandAST) eval(ev *Evaluator) {
	ev.evalCommand(ast)
}

func (ast *commandAST) show() {
	logf("\t%s", strings.Replace(ast.cmd, "\n", `\n`, -1))
}

type includeAST struct {
	astBase
	expr string
	op   string
}

func (ast *includeAST) eval(ev *Evaluator) {
	ev.evalInclude(ast)
}

func (ast *includeAST) show() {
	logf("include %s", ast.expr)
}

type ifAST struct {
	astBase
	op         string
	lhs        Value
	rhs        Value // Empty if |op| is ifdef or ifndef.
	trueStmts  []ast
	falseStmts []ast
}

func (ast *ifAST) eval(ev *Evaluator) {
	ev.evalIf(ast)
}

func (ast *ifAST) show() {
	// TODO
	logf("if")
}

type exportAST struct {
	astBase
	expr   []byte
	export bool
}

func (ast *exportAST) eval(ev *Evaluator) {
	ev.evalExport(ast)
}

func (ast *exportAST) show() {
	// TODO
	logf("export")
}
