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

type srcpos struct {
	filename string
	lineno   int
}

func (p srcpos) String() string {
	return fmt.Sprintf("%s:%d", p.filename, p.lineno)
}

// EvalError is an error in kati evaluation.
type EvalError struct {
	Filename string
	Lineno   int
	Err      error
}

func (e EvalError) Error() string {
	return fmt.Sprintf("%s:%d: %v", e.Filename, e.Lineno, e.Err)
}

func (p srcpos) errorf(f string, args ...interface{}) error {
	return EvalError{
		Filename: p.filename,
		Lineno:   p.lineno,
		Err:      fmt.Errorf(f, args...),
	}
}

func (p srcpos) error(err error) error {
	if _, ok := err.(EvalError); ok {
		return err
	}
	return EvalError{
		Filename: p.filename,
		Lineno:   p.lineno,
		Err:      err,
	}
}

// Evaluator manages makefile evaluation.
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

	srcpos
}

// NewEvaluator creates new Evaluator.
func NewEvaluator(vars map[string]Var) *Evaluator {
	return &Evaluator{
		outVars:     make(Vars),
		vars:        vars,
		outRuleVars: make(map[string]Vars),
		exports:     make(map[string]bool),
	}
}

func (ev *Evaluator) args(buf *buffer, args ...Value) ([][]byte, error) {
	pos := make([]int, 0, len(args))
	for _, arg := range args {
		buf.resetSpace()
		err := arg.Eval(buf, ev)
		if err != nil {
			return nil, err
		}
		pos = append(pos, buf.Len())
	}
	v := buf.Bytes()
	buf.args = buf.args[:0]
	s := 0
	for _, p := range pos {
		buf.args = append(buf.args, v[s:p])
		s = p
	}
	return buf.args, nil
}

func (ev *Evaluator) evalAssign(ast *assignAST) error {
	ev.lastRule = nil
	lhs, rhs, err := ev.evalAssignAST(ast)
	if err != nil {
		return err
	}
	if LogFlag {
		logf("ASSIGN: %s=%q (flavor:%q)", lhs, rhs, rhs.Flavor())
	}
	if lhs == "" {
		return ast.errorf("*** empty variable name.")
	}
	ev.outVars.Assign(lhs, rhs)
	return nil
}

func (ev *Evaluator) evalAssignAST(ast *assignAST) (string, Var, error) {
	ev.srcpos = ast.srcpos

	var lhs string
	switch v := ast.lhs.(type) {
	case literal:
		lhs = string(v)
	case tmpval:
		lhs = string(v)
	default:
		buf := newBuf()
		err := v.Eval(buf, ev)
		if err != nil {
			return "", nil, err
		}
		lhs = string(trimSpaceBytes(buf.Bytes()))
		freeBuf(buf)
	}
	rhs, err := ast.evalRHS(ev, lhs)
	if err != nil {
		return "", nil, err
	}
	return lhs, rhs, nil
}

func (ev *Evaluator) setTargetSpecificVar(assign *assignAST, output string) error {
	vars, present := ev.outRuleVars[output]
	if !present {
		vars = make(Vars)
		ev.outRuleVars[output] = vars
	}
	ev.currentScope = vars
	lhs, rhs, err := ev.evalAssignAST(assign)
	if err != nil {
		return err
	}
	if LogFlag {
		logf("rule outputs:%q assign:%q%s%q (flavor:%q)", output, lhs, assign.op, rhs, rhs.Flavor())
	}
	vars.Assign(lhs, &targetSpecificVar{v: rhs, op: assign.op})
	ev.currentScope = nil
	return nil
}

func (ev *Evaluator) evalMaybeRule(ast *maybeRuleAST) error {
	ev.lastRule = nil
	ev.srcpos = ast.srcpos

	logf("maybe rule %s: %q assign:%v", ev.srcpos, ast.expr, ast.assign)

	abuf := newBuf()
	aexpr := toExpr(ast.expr)
	var rhs expr
	semi := ast.semi
	for i, v := range aexpr {
		var buf buffer
		buf.resetSpace()
		err := v.Eval(&buf, ev)
		if err != nil {
			return err
		}
		b := buf.Bytes()
		if ast.isRule {
			abuf.Write(b)
			continue
		}
		eq := findLiteralChar(b, '=', 0)
		if eq >= 0 {
			abuf.Write(b[:eq+1])
			if eq+1 < len(b) {
				rhs = append(rhs, tmpval(trimLeftSpaceBytes(b[eq+1:])))
			}
			if i+1 < len(aexpr) {
				rhs = append(rhs, aexpr[i+1:]...)
			}
			if ast.semi != nil {
				rhs = append(rhs, literal(';'))
				sexpr, _, err := parseExpr(ast.semi, nil, parseOp{})
				if err != nil {
					return err
				}
				rhs = append(rhs, toExpr(sexpr)...)
				semi = nil
			}
			break
		}
		abuf.Write(b)
	}

	line := abuf.Bytes()
	r := &rule{srcpos: ast.srcpos}
	assign, err := r.parse(line, ast.assign, rhs)
	if err != nil {
		return ast.error(err)
	}
	freeBuf(abuf)
	if LogFlag {
		logf("rule %q assign:%v rhs:%v=> outputs:%q, inputs:%q", ast.expr, ast.assign, rhs, r.outputs, r.inputs)
	}

	// TODO: Pretty print.
	//logf("RULE: %s=%s (%d commands)", lhs, rhs, len(cmds))

	if assign != nil {
		logf("target specific var: %#v", assign)
		for _, output := range r.outputs {
			ev.setTargetSpecificVar(assign, output)
		}
		for _, output := range r.outputPatterns {
			ev.setTargetSpecificVar(assign, output.String())
		}
		return nil
	}

	if semi != nil {
		r.cmds = append(r.cmds, string(semi))
	}
	if LogFlag {
		logf("rule outputs:%q cmds:%q", r.outputs, r.cmds)
	}
	ev.lastRule = r
	ev.outRules = append(ev.outRules, r)
	return nil
}

func (ev *Evaluator) evalCommand(ast *commandAST) error {
	ev.srcpos = ast.srcpos
	if ev.lastRule == nil || ev.lastRule.outputs == nil {
		// This could still be an assignment statement. See
		// assign_after_tab.mk.
		if strings.IndexByte(ast.cmd, '=') >= 0 {
			line := trimLeftSpace(ast.cmd)
			mk, err := parseMakefileString(line, ast.srcpos)
			if err != nil {
				return ast.errorf("parse failed: %q: %v", line, err)
			}
			if len(mk.stmts) >= 1 && mk.stmts[len(mk.stmts)-1].(*assignAST) != nil {
				for _, stmt := range mk.stmts {
					err = ev.eval(stmt)
					if err != nil {
						return err
					}
				}
			}
			return nil
		}
		// Or, a comment is OK.
		if strings.TrimSpace(ast.cmd)[0] == '#' {
			return nil
		}
		return ast.errorf("*** commands commence before first target.")
	}
	ev.lastRule.cmds = append(ev.lastRule.cmds, ast.cmd)
	if ev.lastRule.cmdLineno == 0 {
		ev.lastRule.cmdLineno = ast.lineno
	}
	return nil
}

// LookupVar looks up named variable.
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

func (ev *Evaluator) lookupVarInCurrentScope(name string) Var {
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
func (ev *Evaluator) EvaluateVar(name string) (string, error) {
	var buf buffer
	buf.resetSpace()
	err := ev.LookupVar(name).Eval(&buf, ev)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (ev *Evaluator) evalIncludeFile(fname string, mk makefile) error {
	te := traceEvent.begin("include", literal(fname), traceEventMain)
	defer func() {
		traceEvent.end(te)
	}()
	var err error
	makefileList := ev.outVars.Lookup("MAKEFILE_LIST")
	makefileList, err = makefileList.Append(ev, mk.filename)
	if err != nil {
		return err
	}
	ev.outVars.Assign("MAKEFILE_LIST", makefileList)

	for _, stmt := range mk.stmts {
		err = ev.eval(stmt)
		if err != nil {
			return err
		}
	}
	return nil
}

func (ev *Evaluator) evalInclude(ast *includeAST) error {
	ev.lastRule = nil
	ev.srcpos = ast.srcpos

	logf("%s include %q", ev.srcpos, ast.expr)
	v, _, err := parseExpr([]byte(ast.expr), nil, parseOp{})
	if err != nil {
		return ast.errorf("parse failed: %q: %v", ast.expr, err)
	}
	var buf buffer
	buf.resetSpace()
	err = v.Eval(&buf, ev)
	if err != nil {
		return ast.errorf("%v", err)
	}
	pats := splitSpaces(buf.String())
	buf.Reset()

	var files []string
	for _, pat := range pats {
		if strings.Contains(pat, "*") || strings.Contains(pat, "?") {
			matched, err := filepath.Glob(pat)
			if err != nil {
				return ast.errorf("glob error: %s: %v", pat, err)
			}
			files = append(files, matched...)
		} else {
			files = append(files, pat)
		}
	}

	for _, fn := range files {
		fn = trimLeadingCurdir(fn)
		if IgnoreOptionalInclude != "" && ast.op == "-include" && matchPattern(fn, IgnoreOptionalInclude) {
			continue
		}
		mk, hash, err := makefileCache.parse(fn)
		if os.IsNotExist(err) {
			if ast.op == "include" {
				return ev.errorf("%v\nNOTE: kati does not support generating missing makefiles", err)
			}
			msg := ev.cache.update(fn, hash, fileNotExists)
			if msg != "" {
				warn(ev.srcpos, "%s", msg)
			}
			continue
		}
		msg := ev.cache.update(fn, hash, fileExists)
		if msg != "" {
			warn(ev.srcpos, "%s", msg)
		}
		err = ev.evalIncludeFile(fn, mk)
		if err != nil {
			return err
		}
	}
	return nil
}

func (ev *Evaluator) evalIf(iast *ifAST) error {
	var isTrue bool
	switch iast.op {
	case "ifdef", "ifndef":
		expr := iast.lhs
		buf := newBuf()
		err := expr.Eval(buf, ev)
		if err != nil {
			return iast.errorf("%v\n expr:%s", err, expr)
		}
		v := ev.LookupVar(buf.String())
		buf.Reset()
		err = v.Eval(buf, ev)
		if err != nil {
			return iast.errorf("%v\n expr:%s=>%s", err, expr, v)
		}
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
		params, err := ev.args(buf, lexpr, rexpr)
		if err != nil {
			return iast.errorf("%v\n (%s,%s)", err, lexpr, rexpr)
		}
		lhs := string(params[0])
		rhs := string(params[1])
		freeBuf(buf)
		isTrue = (lhs == rhs) == (iast.op == "ifeq")
		if LogFlag {
			logf("%s lhs=%q %q rhs=%q %q => %t", iast.op, iast.lhs, lhs, iast.rhs, rhs, isTrue)
		}
	default:
		return iast.errorf("unknown if statement: %q", iast.op)
	}

	var stmts []ast
	if isTrue {
		stmts = iast.trueStmts
	} else {
		stmts = iast.falseStmts
	}
	for _, stmt := range stmts {
		err := ev.eval(stmt)
		if err != nil {
			return err
		}
	}
	return nil
}

func (ev *Evaluator) evalExport(ast *exportAST) error {
	ev.lastRule = nil
	ev.srcpos = ast.srcpos

	v, _, err := parseExpr(ast.expr, nil, parseOp{})
	if err != nil {
		return ast.errorf("failed to parse: %q: %v", string(ast.expr), err)
	}
	var buf buffer
	buf.resetSpace()
	err = v.Eval(&buf, ev)
	if err != nil {
		return ast.errorf("%v\n expr:%s", err, v)
	}
	for _, n := range splitSpacesBytes(buf.Bytes()) {
		ev.exports[string(n)] = ast.export
	}
	return nil
}

func (ev *Evaluator) eval(stmt ast) error {
	return stmt.eval(ev)
}

func eval(mk makefile, vars Vars, useCache bool) (er *evalResult, err error) {
	ev := NewEvaluator(vars)
	if useCache {
		ev.cache = newAccessCache()
	}

	makefileList := vars.Lookup("MAKEFILE_LIST")
	if !makefileList.IsDefined() {
		makefileList = &simpleVar{value: "", origin: "file"}
	}
	makefileList, err = makefileList.Append(ev, mk.filename)
	if err != nil {
		return nil, err
	}
	ev.outVars.Assign("MAKEFILE_LIST", makefileList)

	for _, stmt := range mk.stmts {
		err = ev.eval(stmt)
		if err != nil {
			return nil, err
		}
	}

	return &evalResult{
		vars:        ev.outVars,
		rules:       ev.outRules,
		ruleVars:    ev.outRuleVars,
		accessedMks: ev.cache.Slice(),
		exports:     ev.exports,
	}, nil
}
