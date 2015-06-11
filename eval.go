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

package main

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
)

type FileState int

const (
	FileExists FileState = iota
	FileNotExists
	FileInconsistent // Modified during kati is running.
)

type ReadMakefile struct {
	Filename string
	Hash     [sha1.Size]byte
	State    FileState
}

type EvalResult struct {
	vars     Vars
	rules    []*Rule
	ruleVars map[string]Vars
	readMks  []*ReadMakefile
	exports  map[string]bool
}

type Evaluator struct {
	paramVars    []tmpval // $1 => paramVars[1]
	outVars      Vars
	outRules     []*Rule
	outRuleVars  map[string]Vars
	vars         Vars
	lastRule     *Rule
	currentScope Vars
	avoidIO      bool
	hasIO        bool
	readMks      map[string]*ReadMakefile
	exports      map[string]bool

	filename string
	lineno   int
}

func newEvaluator(vars map[string]Var) *Evaluator {
	return &Evaluator{
		outVars:     make(Vars),
		vars:        vars,
		outRuleVars: make(map[string]Vars),
		readMks:     make(map[string]*ReadMakefile),
		exports:     make(map[string]bool),
	}
}

func (ev *Evaluator) args(buf *buffer, args ...Value) [][]byte {
	var pos []int
	for _, arg := range args {
		arg.Eval(buf, ev)
		pos = append(pos, buf.Len())
	}
	v := buf.Bytes()
	buf.args = buf.args[:0]
	s := 0
	for _, p := range pos {
		buf.args = append(buf.args, v[s:p])
		s = p
	}
	return buf.args
}

func (ev *Evaluator) evalAssign(ast *AssignAST) {
	ev.lastRule = nil
	lhs, rhs := ev.evalAssignAST(ast)
	Logf("ASSIGN: %s=%q (flavor:%q)", lhs, rhs, rhs.Flavor())
	if len(lhs) == 0 {
		Error(ast.filename, ast.lineno, "*** empty variable name.")
	}
	ev.outVars.Assign(lhs, rhs)
}

func (ev *Evaluator) evalAssignAST(ast *AssignAST) (string, Var) {
	ev.filename = ast.filename
	ev.lineno = ast.lineno

	var lhs string
	switch v := ast.lhs.(type) {
	case literal:
		lhs = string(v)
	case tmpval:
		lhs = string(v)
	default:
		buf := newBuf()
		v.Eval(buf, ev)
		lhs = string(trimSpaceBytes(buf.Bytes()))
		freeBuf(buf)
	}
	rhs := ast.evalRHS(ev, lhs)
	return lhs, rhs
}

func (ev *Evaluator) setTargetSpecificVar(assign *AssignAST, output string) {
	vars, present := ev.outRuleVars[output]
	if !present {
		vars = make(Vars)
		ev.outRuleVars[output] = vars
	}
	ev.currentScope = vars
	lhs, rhs := ev.evalAssignAST(assign)
	Logf("rule outputs:%q assign:%q=%q (flavor:%q)", output, lhs, rhs, rhs.Flavor())
	vars.Assign(lhs, TargetSpecificVar{v: rhs, op: assign.op})
	ev.currentScope = nil
}

func (ev *Evaluator) evalMaybeRule(ast *MaybeRuleAST) {
	ev.lastRule = nil
	ev.filename = ast.filename
	ev.lineno = ast.lineno

	lexpr := ast.expr
	buf := newBuf()
	lexpr.Eval(buf, ev)
	line := buf.Bytes()
	if ast.term == '=' {
		line = append(line, ast.afterTerm...)
	}
	Logf("rule? %q=>%q", ast.expr, line)

	// See semicolon.mk.
	if len(bytes.TrimRight(line, " \t\n;")) == 0 {
		freeBuf(buf)
		return
	}

	rule := &Rule{
		filename: ast.filename,
		lineno:   ast.lineno,
	}
	assign, err := rule.parse(line)
	if err != nil {
		Error(ast.filename, ast.lineno, "%v", err.Error())
	}
	freeBuf(buf)
	Logf("rule %q => outputs:%q, inputs:%q", line, rule.outputs, rule.inputs)

	// TODO: Pretty print.
	//Logf("RULE: %s=%s (%d commands)", lhs, rhs, len(cmds))

	if assign != nil {
		if ast.term == ';' {
			nexpr, _, err := parseExpr(ast.afterTerm, nil)
			if err != nil {
				panic(fmt.Errorf("parse %s:%d %v", ev.filename, ev.lineno, err))
			}
			lexpr = Expr{lexpr, nexpr}

			buf = newBuf()
			lexpr.Eval(buf, ev)
			assign, err = rule.parse(buf.Bytes())
			if err != nil {
				Error(ast.filename, ast.lineno, "%v", err.Error())
			}
			freeBuf(buf)
		}
		for _, output := range rule.outputs {
			ev.setTargetSpecificVar(assign, output)
		}
		for _, output := range rule.outputPatterns {
			ev.setTargetSpecificVar(assign, output.String())
		}
		return
	}

	if ast.term == ';' {
		rule.cmds = append(rule.cmds, string(ast.afterTerm[1:]))
	}
	Logf("rule outputs:%q cmds:%q", rule.outputs, rule.cmds)
	ev.lastRule = rule
	ev.outRules = append(ev.outRules, rule)
}

func (ev *Evaluator) evalCommand(ast *CommandAST) {
	ev.filename = ast.filename
	ev.lineno = ast.lineno
	if ev.lastRule == nil {
		// This could still be an assignment statement. See
		// assign_after_tab.mk.
		if strings.IndexByte(ast.cmd, '=') >= 0 {
			line := trimLeftSpace(ast.cmd)
			mk, err := ParseMakefileString(line, ast.filename, ast.lineno)
			if err != nil {
				panic(err)
			}
			if len(mk.stmts) == 1 && mk.stmts[0].(*AssignAST) != nil {
				ev.eval(mk.stmts[0])
			}
			return
		}
		// Or, a comment is OK.
		if strings.TrimSpace(ast.cmd)[0] == '#' {
			return
		}
		Error(ast.filename, ast.lineno, "*** commands commence before first target.")
	}
	ev.lastRule.cmds = append(ev.lastRule.cmds, ast.cmd)
	if ev.lastRule.cmdLineno == 0 {
		ev.lastRule.cmdLineno = ast.lineno
	}
}

func (ev *Evaluator) LookupVar(name string) Var {
	if ev.currentScope != nil {
		v := ev.currentScope.Lookup(name)
		if v.IsDefined() {
			return v
		}
	}
	v := ev.outVars.Lookup(name)
	if v.IsDefined() {
		return v
	}
	return ev.vars.Lookup(name)
}

func (ev *Evaluator) LookupVarInCurrentScope(name string) Var {
	if ev.currentScope != nil {
		v := ev.currentScope.Lookup(name)
		return v
	}
	v := ev.outVars.Lookup(name)
	if v.IsDefined() {
		return v
	}
	return ev.vars.Lookup(name)
}

// EvaluateVar evaluates variable named name.
// Only for a few special uses such as getting SHELL and handling
// export/unexport.
func (ev *Evaluator) EvaluateVar(name string) string {
	var buf bytes.Buffer
	ev.LookupVar(name).Eval(&buf, ev)
	return buf.String()
}

func (ev *Evaluator) evalIncludeFile(fname string, c []byte) error {
	te := traceEvent.begin("include", literal(fname))
	defer func() {
		traceEvent.end(te)
	}()
	mk, ok, err := LookupMakefileCache(fname)
	if !ok {
		Logf("Reading makefile %q", fname)
		mk, err = ParseMakefile(c, fname)
	}
	if err != nil {
		return err
	}
	makefileList := ev.outVars.Lookup("MAKEFILE_LIST")
	makefileList = makefileList.Append(ev, mk.filename)
	ev.outVars.Assign("MAKEFILE_LIST", makefileList)

	for _, stmt := range mk.stmts {
		ev.eval(stmt)
	}
	return nil
}

func (ev *Evaluator) updateReadMakefile(fn string, c []byte, st FileState) {
	if !useCache {
		return
	}

	h := sha1.Sum(c)
	rm, present := ev.readMks[fn]
	if present {
		switch rm.State {
		case FileExists:
			if st != FileExists {
				Warn(ev.filename, ev.lineno, "%s was removed after the previous read", fn)
			} else if !bytes.Equal(h[:], rm.Hash[:]) {
				Warn(ev.filename, ev.lineno, "%s was modified after the previous read", fn)
				ev.readMks[fn].State = FileInconsistent
			}
			return
		case FileNotExists:
			if st != FileNotExists {
				Warn(ev.filename, ev.lineno, "%s was created after the previous read", fn)
				ev.readMks[fn].State = FileInconsistent
			}
		case FileInconsistent:
			return
		}
	} else {
		ev.readMks[fn] = &ReadMakefile{
			Filename: fn,
			Hash:     h,
			State:    st,
		}
	}
}

func (ev *Evaluator) evalInclude(ast *IncludeAST) {
	ev.lastRule = nil
	ev.filename = ast.filename
	ev.lineno = ast.lineno

	Logf("%s:%d include %q", ev.filename, ev.lineno, ast.expr)
	v, _, err := parseExpr([]byte(ast.expr), nil)
	if err != nil {
		panic(err)
	}
	var buf bytes.Buffer
	v.Eval(&buf, ev)
	pats := splitSpaces(buf.String())
	buf.Reset()

	var files []string
	for _, pat := range pats {
		if strings.Contains(pat, "*") || strings.Contains(pat, "?") {
			matched, err := filepath.Glob(pat)
			if err != nil {
				panic(err)
			}
			files = append(files, matched...)
		} else {
			files = append(files, pat)
		}
	}

	for _, fn := range files {
		if ignoreOptionalInclude != "" && ast.op == "-include" && matchPattern(fn, ignoreOptionalInclude) {
			continue
		}
		c, err := ioutil.ReadFile(fn)
		if err != nil {
			if ast.op == "include" {
				Error(ev.filename, ev.lineno, fmt.Sprintf("%v\nNOTE: kati does not support generating missing makefiles", err))
			} else {
				ev.updateReadMakefile(fn, nil, FileNotExists)
				continue
			}
		}
		ev.updateReadMakefile(fn, c, FileExists)
		err = ev.evalIncludeFile(fn, c)
		if err != nil {
			panic(err)
		}
	}
}

func (ev *Evaluator) evalIf(ast *IfAST) {
	var isTrue bool
	switch ast.op {
	case "ifdef", "ifndef":
		expr := ast.lhs
		buf := newBuf()
		expr.Eval(buf, ev)
		v := ev.LookupVar(buf.String())
		buf.Reset()
		v.Eval(buf, ev)
		value := buf.String()
		val := buf.Len()
		freeBuf(buf)
		isTrue = (val > 0) == (ast.op == "ifdef")
		Logf("%s lhs=%q value=%q => %t", ast.op, ast.lhs, value, isTrue)
	case "ifeq", "ifneq":
		lexpr := ast.lhs
		rexpr := ast.rhs
		buf := newBuf()
		params := ev.args(buf, lexpr, rexpr)
		lhs := string(params[0])
		rhs := string(params[1])
		freeBuf(buf)
		isTrue = (lhs == rhs) == (ast.op == "ifeq")
		Logf("%s lhs=%q %q rhs=%q %q => %t", ast.op, ast.lhs, lhs, ast.rhs, rhs, isTrue)
	default:
		panic(fmt.Sprintf("unknown if statement: %q", ast.op))
	}

	var stmts []AST
	if isTrue {
		stmts = ast.trueStmts
	} else {
		stmts = ast.falseStmts
	}
	for _, stmt := range stmts {
		ev.eval(stmt)
	}
}

func (ev *Evaluator) evalExport(ast *ExportAST) {
	ev.lastRule = nil
	ev.filename = ast.filename
	ev.lineno = ast.lineno

	v, _, err := parseExpr(ast.expr, nil)
	if err != nil {
		panic(err)
	}
	var buf bytes.Buffer
	v.Eval(&buf, ev)
	for _, n := range splitSpacesBytes(buf.Bytes()) {
		ev.exports[string(n)] = ast.export
	}
}

func (ev *Evaluator) eval(ast AST) {
	ast.eval(ev)
}

func createReadMakefileArray(mp map[string]*ReadMakefile) []*ReadMakefile {
	var r []*ReadMakefile
	for _, v := range mp {
		r = append(r, v)
	}
	return r
}

func Eval(mk Makefile, vars Vars) (er *EvalResult, err error) {
	ev := newEvaluator(vars)
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in eval %s: %v", mk.filename, r)
		}
	}()

	makefileList := vars.Lookup("MAKEFILE_LIST")
	makefileList = makefileList.Append(ev, mk.filename)
	ev.outVars.Assign("MAKEFILE_LIST", makefileList)

	for _, stmt := range mk.stmts {
		ev.eval(stmt)
	}

	return &EvalResult{
		vars:     ev.outVars,
		rules:    ev.outRules,
		ruleVars: ev.outRuleVars,
		readMks:  createReadMakefileArray(ev.readMks),
		exports:  ev.exports,
	}, nil
}
