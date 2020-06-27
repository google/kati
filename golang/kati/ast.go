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
	"strings"

	"github.com/golang/glog"
)

type ast interface {
	eval(*Evaluator) error
	show()
}

type assignAST struct {
	srcpos
	lhs Value
	rhs Value
	op  string
	opt string // "override", "export"
}

func (ast *assignAST) eval(ev *Evaluator) error {
	return ev.evalAssign(ast)
}

func (ast *assignAST) evalRHS(ev *Evaluator, lhs string) (Var, error) {
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
			return &simpleVar{value: []string{v.String()}, origin: origin}, nil
		case tmpval:
			return &simpleVar{value: []string{v.String()}, origin: origin}, nil
		default:
			var buf evalBuffer
			buf.resetSep()
			err := v.Eval(&buf, ev)
			if err != nil {
				return nil, err
			}
			return &simpleVar{value: []string{buf.String()}, origin: origin}, nil
		}
	case "=":
		return &recursiveVar{expr: ast.rhs, origin: origin}, nil
	case "+=":
		prev := ev.lookupVarInCurrentScope(lhs)
		if !prev.IsDefined() {
			return &recursiveVar{expr: ast.rhs, origin: origin}, nil
		}
		return prev.AppendVar(ev, ast.rhs)
	case "?=":
		prev := ev.lookupVarInCurrentScope(lhs)
		if prev.IsDefined() {
			return prev, nil
		}
		return &recursiveVar{expr: ast.rhs, origin: origin}, nil
	}
	return nil, ast.errorf("unknown assign op: %q", ast.op)
}

func (ast *assignAST) show() {
	glog.Infof("%s %s %s %q", ast.opt, ast.lhs, ast.op, ast.rhs)
}

// maybeRuleAST is an ast for rule line.
// Note we cannot be sure what this is, until all variables in |expr|
// are expanded.
type maybeRuleAST struct {
	srcpos
	isRule bool // found literal ':'
	expr   Value
	assign *assignAST // target specific var
	semi   []byte     // after ';' if ';' exists
}

func (ast *maybeRuleAST) eval(ev *Evaluator) error {
	return ev.evalMaybeRule(ast)
}

func (ast *maybeRuleAST) show() {
	glog.Info(ast.expr)
}

type commandAST struct {
	srcpos
	cmd string
}

func (ast *commandAST) eval(ev *Evaluator) error {
	return ev.evalCommand(ast)
}

func (ast *commandAST) show() {
	glog.Infof("\t%s", strings.Replace(ast.cmd, "\n", `\n`, -1))
}

type includeAST struct {
	srcpos
	expr string
	op   string
}

func (ast *includeAST) eval(ev *Evaluator) error {
	return ev.evalInclude(ast)
}

func (ast *includeAST) show() {
	glog.Infof("include %s", ast.expr)
}

type ifAST struct {
	srcpos
	op         string
	lhs        Value
	rhs        Value // Empty if |op| is ifdef or ifndef.
	trueStmts  []ast
	falseStmts []ast
}

func (ast *ifAST) eval(ev *Evaluator) error {
	return ev.evalIf(ast)
}

func (ast *ifAST) show() {
	// TODO
	glog.Info("if")
}

type exportAST struct {
	srcpos
	expr     []byte
	hasEqual bool
	export   bool
}

func (ast *exportAST) eval(ev *Evaluator) error {
	return ev.evalExport(ast)
}

func (ast *exportAST) show() {
	// TODO
	glog.Info("export")
}

type vpathAST struct {
	srcpos
	expr Value
}

func (ast *vpathAST) eval(ev *Evaluator) error {
	return ev.evalVpath(ast)
}

func (ast *vpathAST) show() {
	glog.Infof("vpath %s", ast.expr.String())
}
