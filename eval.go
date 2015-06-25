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
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type fileState int

const (
	fileExists fileState = iota
	fileNotExists
	fileInconsistent // Modified during kati is running.
)

type accessedMakefile struct {
	Filename string
	Hash     [sha1.Size]byte
	State    fileState
}

type accessCache struct {
	mu sync.Mutex
	m  map[string]*accessedMakefile
}

func newAccessCache() *accessCache {
	return &accessCache{
		m: make(map[string]*accessedMakefile),
	}
}

func (ac *accessCache) update(fn string, hash [sha1.Size]byte, st fileState) string {
	if ac == nil {
		return ""
	}
	ac.mu.Lock()
	defer ac.mu.Unlock()
	rm, present := ac.m[fn]
	if present {
		switch rm.State {
		case fileExists:
			if st != fileExists {
				return fmt.Sprintf("%s was removed after the previous read", fn)
			} else if !bytes.Equal(hash[:], rm.Hash[:]) {
				ac.m[fn].State = fileInconsistent
				return fmt.Sprintf("%s was modified after the previous read", fn)
			}
			return ""
		case fileNotExists:
			if st != fileNotExists {
				ac.m[fn].State = fileInconsistent
				return fmt.Sprintf("%s was created after the previous read", fn)
			}
		case fileInconsistent:
			return ""
		}
		return ""
	}
	ac.m[fn] = &accessedMakefile{
		Filename: fn,
		Hash:     hash,
		State:    st,
	}
	return ""
}

func (ac *accessCache) Slice() []*accessedMakefile {
	if ac == nil {
		return nil
	}
	ac.mu.Lock()
	defer ac.mu.Unlock()
	r := []*accessedMakefile{}
	for _, v := range ac.m {
		r = append(r, v)
	}
	return r
}

type evalResult struct {
	vars        Vars
	rules       []*rule
	ruleVars    map[string]Vars
	accessedMks []*accessedMakefile
	exports     map[string]bool
}

type Evaluator struct {
	paramVars    []tmpval // $1 => paramVars[1]
	outVars      Vars
	outRules     []*rule
	outRuleVars  map[string]Vars
	vars         Vars
	lastRule     *rule
	currentScope Vars
	avoidIO      bool
	hasIO        bool
	cache        *accessCache
	exports      map[string]bool

	filename string
	lineno   int
}

func NewEvaluator(vars map[string]Var) *Evaluator {
	return &Evaluator{
		outVars:     make(Vars),
		vars:        vars,
		outRuleVars: make(map[string]Vars),
		exports:     make(map[string]bool),
	}
}

func (ev *Evaluator) args(buf *buffer, args ...Value) [][]byte {
	pos := make([]int, 0, len(args))
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

func (ev *Evaluator) evalAssign(ast *assignAST) {
	ev.lastRule = nil
	lhs, rhs := ev.evalAssignAST(ast)
	if LogFlag {
		logf("ASSIGN: %s=%q (flavor:%q)", lhs, rhs, rhs.Flavor())
	}
	if lhs == "" {
		errorExit(ast.filename, ast.lineno, "*** empty variable name.")
	}
	ev.outVars.Assign(lhs, rhs)
}

func (ev *Evaluator) evalAssignAST(ast *assignAST) (string, Var) {
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

func (ev *Evaluator) setTargetSpecificVar(assign *assignAST, output string) {
	vars, present := ev.outRuleVars[output]
	if !present {
		vars = make(Vars)
		ev.outRuleVars[output] = vars
	}
	ev.currentScope = vars
	lhs, rhs := ev.evalAssignAST(assign)
	if LogFlag {
		logf("rule outputs:%q assign:%q=%q (flavor:%q)", output, lhs, rhs, rhs.Flavor())
	}
	vars.Assign(lhs, &targetSpecificVar{v: rhs, op: assign.op})
	ev.currentScope = nil
}

func (ev *Evaluator) evalMaybeRule(ast *maybeRuleAST) {
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
	if LogFlag {
		logf("rule? %q=>%q", ast.expr, line)
	}

	// See semicolon.mk.
	if len(bytes.TrimRight(line, " \t\n;")) == 0 {
		freeBuf(buf)
		return
	}

	r := &rule{
		filename: ast.filename,
		lineno:   ast.lineno,
	}
	assign, err := r.parse(line)
	if err != nil {
		errorExit(ast.filename, ast.lineno, "%v", err)
	}
	freeBuf(buf)
	if LogFlag {
		logf("rule %q => outputs:%q, inputs:%q", line, r.outputs, r.inputs)
	}

	// TODO: Pretty print.
	//logf("RULE: %s=%s (%d commands)", lhs, rhs, len(cmds))

	if assign != nil {
		if ast.term == ';' {
			nexpr, _, err := parseExpr(ast.afterTerm, nil, false)
			if err != nil {
				panic(fmt.Errorf("parse %s:%d %v", ev.filename, ev.lineno, err))
			}
			lexpr = expr{lexpr, nexpr}

			buf = newBuf()
			lexpr.Eval(buf, ev)
			assign, err = r.parse(buf.Bytes())
			if err != nil {
				errorExit(ast.filename, ast.lineno, "%v", err)
			}
			freeBuf(buf)
		}
		for _, output := range r.outputs {
			ev.setTargetSpecificVar(assign, output)
		}
		for _, output := range r.outputPatterns {
			ev.setTargetSpecificVar(assign, output.String())
		}
		return
	}

	if ast.term == ';' {
		r.cmds = append(r.cmds, string(ast.afterTerm[1:]))
	}
	if LogFlag {
		logf("rule outputs:%q cmds:%q", r.outputs, r.cmds)
	}
	ev.lastRule = r
	ev.outRules = append(ev.outRules, r)
}

func (ev *Evaluator) evalCommand(ast *commandAST) {
	ev.filename = ast.filename
	ev.lineno = ast.lineno
	if ev.lastRule == nil {
		// This could still be an assignment statement. See
		// assign_after_tab.mk.
		if strings.IndexByte(ast.cmd, '=') >= 0 {
			line := trimLeftSpace(ast.cmd)
			mk, err := parseMakefileString(line, ast.filename, ast.lineno)
			if err != nil {
				panic(err)
			}
			if len(mk.stmts) == 1 && mk.stmts[0].(*assignAST) != nil {
				ev.eval(mk.stmts[0])
			}
			return
		}
		// Or, a comment is OK.
		if strings.TrimSpace(ast.cmd)[0] == '#' {
			return
		}
		errorExit(ast.filename, ast.lineno, "*** commands commence before first target.")
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

func (ev *Evaluator) evalIncludeFile(fname string, mk makefile) error {
	te := traceEvent.begin("include", literal(fname), traceEventMain)
	defer func() {
		traceEvent.end(te)
	}()
	makefileList := ev.outVars.Lookup("MAKEFILE_LIST")
	makefileList = makefileList.Append(ev, mk.filename)
	ev.outVars.Assign("MAKEFILE_LIST", makefileList)

	for _, stmt := range mk.stmts {
		ev.eval(stmt)
	}
	return nil
}

func (ev *Evaluator) evalInclude(ast *includeAST) {
	ev.lastRule = nil
	ev.filename = ast.filename
	ev.lineno = ast.lineno

	logf("%s:%d include %q", ev.filename, ev.lineno, ast.expr)
	v, _, err := parseExpr([]byte(ast.expr), nil, false)
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
		if IgnoreOptionalInclude != "" && ast.op == "-include" && matchPattern(fn, IgnoreOptionalInclude) {
			continue
		}
		mk, hash, err := makefileCache.parse(fn)
		if os.IsNotExist(err) {
			if ast.op == "include" {
				errorExit(ev.filename, ev.lineno, "%v\nNOTE: kati does not support generating missing makefiles", err)
			} else {
				msg := ev.cache.update(fn, hash, fileNotExists)
				if msg != "" {
					warn(ev.filename, ev.lineno, "%s", msg)
				}
				continue
			}
		}
		msg := ev.cache.update(fn, hash, fileExists)
		if msg != "" {
			warn(ev.filename, ev.lineno, "%s", msg)
		}
		err = ev.evalIncludeFile(fn, mk)
		if err != nil {
			panic(err)
		}
	}
}

func (ev *Evaluator) evalIf(iast *ifAST) {
	var isTrue bool
	switch iast.op {
	case "ifdef", "ifndef":
		expr := iast.lhs
		buf := newBuf()
		expr.Eval(buf, ev)
		v := ev.LookupVar(buf.String())
		buf.Reset()
		v.Eval(buf, ev)
		value := buf.String()
		val := buf.Len()
		freeBuf(buf)
		isTrue = (val > 0) == (iast.op == "ifdef")
		if LogFlag {
			logf("%s lhs=%q value=%q => %t", iast.op, iast.lhs, value, isTrue)
		}
	case "ifeq", "ifneq":
		lexpr := iast.lhs
		rexpr := iast.rhs
		buf := newBuf()
		params := ev.args(buf, lexpr, rexpr)
		lhs := string(params[0])
		rhs := string(params[1])
		freeBuf(buf)
		isTrue = (lhs == rhs) == (iast.op == "ifeq")
		if LogFlag {
			logf("%s lhs=%q %q rhs=%q %q => %t", iast.op, iast.lhs, lhs, iast.rhs, rhs, isTrue)
		}
	default:
		panic(fmt.Sprintf("unknown if statement: %q", iast.op))
	}

	var stmts []ast
	if isTrue {
		stmts = iast.trueStmts
	} else {
		stmts = iast.falseStmts
	}
	for _, stmt := range stmts {
		ev.eval(stmt)
	}
}

func (ev *Evaluator) evalExport(ast *exportAST) {
	ev.lastRule = nil
	ev.filename = ast.filename
	ev.lineno = ast.lineno

	v, _, err := parseExpr(ast.expr, nil, false)
	if err != nil {
		panic(err)
	}
	var buf bytes.Buffer
	v.Eval(&buf, ev)
	for _, n := range splitSpacesBytes(buf.Bytes()) {
		ev.exports[string(n)] = ast.export
	}
}

func (ev *Evaluator) eval(stmt ast) {
	stmt.eval(ev)
}

func eval(mk makefile, vars Vars, useCache bool) (er *evalResult, err error) {
	ev := NewEvaluator(vars)
	if useCache {
		ev.cache = newAccessCache()
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in eval %s: %v", mk.filename, r)
		}
	}()

	makefileList := vars.Lookup("MAKEFILE_LIST")
	if !makefileList.IsDefined() {
		makefileList = &simpleVar{value: "", origin: "file"}
	}
	makefileList = makefileList.Append(ev, mk.filename)
	ev.outVars.Assign("MAKEFILE_LIST", makefileList)

	for _, stmt := range mk.stmts {
		ev.eval(stmt)
	}

	return &evalResult{
		vars:        ev.outVars,
		rules:       ev.outRules,
		ruleVars:    ev.outRuleVars,
		accessedMks: ev.cache.Slice(),
		exports:     ev.exports,
	}, nil
}
