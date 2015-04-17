package main

import (
	"bytes"
	"fmt"
	"strings"
)

type EvalResult struct {
	vars     Vars
	rules    []*Rule
	ruleVars map[string]Vars
}

type Evaluator struct {
	outVars      Vars
	outRules     []*Rule
	outRuleVars  map[string]Vars
	vars         Vars
	lastRule     *Rule
	currentScope Vars

	filename string
	lineno   int
}

func newEvaluator(vars map[string]Var) *Evaluator {
	return &Evaluator{
		outVars:     make(Vars),
		vars:        vars,
		outRuleVars: make(map[string]Vars),
	}
}

func (ev *Evaluator) Value(v Value) []byte {
	if v, ok := v.(Valuer); ok {
		return v.Value()
	}
	var buf bytes.Buffer
	v.Eval(&buf, ev)
	return buf.Bytes()
}

// TODO(ukai): use unicode.IsSpace?
func isWhitespace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r':
		return true
	}
	return false
}

func (ev *Evaluator) Values(v Value) [][]byte {
	var buf bytes.Buffer
	v.Eval(&buf, ev)
	val := buf.Bytes()
	var values [][]byte
	b := -1
	for i := 0; i < len(val); i++ {
		if b < 0 {
			if isWhitespace(val[i]) {
				continue
			}
			b = i
		} else {
			if isWhitespace(val[i]) {
				values = append(values, val[b:i])
				b = -1
				continue
			}
		}
	}
	if b >= 0 {
		values = append(values, val[b:])
	}
	return values
}

// TODO(ukai): deprecated.
func (ev *Evaluator) evalExprBytes(ex string) []byte {
	v, _, err := parseExpr([]byte(ex), nil)
	if err != nil {
		panic(err)
	}
	return ev.Value(v)
}

func (ev *Evaluator) evalExpr(ex string) string {
	return string(ev.evalExprBytes(ex))
}

func (ev *Evaluator) evalAssign(ast *AssignAST) {
	ev.lastRule = nil
	lhs, rhs := ev.evalAssignAST(ast)
	Log("ASSIGN: %s=%q (flavor:%q)", lhs, rhs, rhs.Flavor())
	if len(lhs) == 0 {
		Error(ast.filename, ast.lineno, "*** empty variable name.")
	}
	ev.outVars.Assign(lhs, rhs)
}

func (ev *Evaluator) evalAssignAST(ast *AssignAST) (string, Var) {
	ev.filename = ast.filename
	ev.lineno = ast.lineno

	lhs := strings.TrimSpace(ev.evalExpr(ast.lhs))
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
	Log("rule outputs:%q assign:%q=%q (flavor:%q)", output, lhs, rhs, rhs.Flavor())
	vars.Assign(lhs, TargetSpecificVar{v: rhs, op: assign.op})
	ev.currentScope = nil
}

func (ev *Evaluator) evalMaybeRule(ast *MaybeRuleAST) {
	ev.lastRule = nil
	ev.filename = ast.filename
	ev.lineno = ast.lineno

	expr := ast.expr
	if ast.semicolonIndex >= 0 {
		expr = expr[0:ast.semicolonIndex]
	}
	if ast.equalIndex >= 0 {
		expr = expr[0:ast.equalIndex]
	}
	line := ev.evalExpr(expr)
	if ast.equalIndex >= 0 {
		line += ast.expr[ast.equalIndex:]
	}
	Log("rule? %q=>%q", ast.expr, line)

	// See semicolon.mk.
	if strings.TrimRight(line, " \t\n;") == "" {
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
	Log("rule %q => outputs:%q, inputs:%q", line, rule.outputs, rule.inputs)

	// TODO: Pretty print.
	//Log("RULE: %s=%s (%d commands)", lhs, rhs, len(cmds))

	if assign != nil {
		if ast.semicolonIndex >= 0 {
			assign, err = rule.parse(ev.evalExpr(ast.expr))
			if err != nil {
				Error(ast.filename, ast.lineno, "%v", err.Error())
			}
		}
		for _, output := range rule.outputs {
			ev.setTargetSpecificVar(assign, output)
		}
		for _, output := range rule.outputPatterns {
			ev.setTargetSpecificVar(assign, output)
		}
		return
	}

	if ast.semicolonIndex > 0 {
		rule.cmds = append(rule.cmds, ast.expr[ast.semicolonIndex+1:])
	}
	Log("rule outputs:%q cmds:%q", rule.outputs, rule.cmds)
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

func (ev *Evaluator) evalInclude(ast *IncludeAST) {
	ev.lastRule = nil
	ev.filename = ast.filename
	ev.lineno = ast.lineno

	Log("%s:%d include %q", ev.filename, ev.lineno, ast.expr)
	// TODO: Handle glob
	v, _, err := parseExpr([]byte(ast.expr), nil)
	if err != nil {
		panic(err)
	}
	files := ev.Values(v)
	for _, f := range files {
		file := string(f)
		Log("Reading makefile %q", file)
		mk, err := ParseMakefile(file)
		if err != nil {
			if ast.op == "include" {
				panic(err)
			} else {
				continue
			}
		}

		makefileList := ev.outVars.Lookup("MAKEFILE_LIST")
		makefileList = makefileList.Append(ev, mk.filename)
		ev.outVars.Assign("MAKEFILE_LIST", makefileList)

		for _, stmt := range mk.stmts {
			ev.eval(stmt)
		}
	}
}

func (ev *Evaluator) evalIf(ast *IfAST) {
	var isTrue bool
	switch ast.op {
	case "ifdef", "ifndef":
		vname := ev.evalExpr(ast.lhs)
		v := ev.LookupVar(string(vname))
		value := ev.Value(v)
		isTrue = (len(value) > 0) == (ast.op == "ifdef")
		Log("%s lhs=%q value=%q => %t", ast.op, ast.lhs, value, isTrue)
	case "ifeq", "ifneq":
		lhs := ev.evalExpr(ast.lhs)
		rhs := ev.evalExpr(ast.rhs)
		isTrue = (lhs == rhs) == (ast.op == "ifeq")
		Log("%s lhs=%q %q rhs=%q %q => %t", ast.op, ast.lhs, lhs, ast.rhs, rhs, isTrue)
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

func (ev *Evaluator) eval(ast AST) {
	ast.eval(ev)
}

func Eval(mk Makefile, vars Vars) (er *EvalResult, err error) {
	ev := newEvaluator(vars)
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in eval %s: %v", mk.filename, r)
		}
	}()

	makefile_list := vars.Lookup("MAKEFILE_LIST")
	makefile_list = makefile_list.Append(ev, mk.filename)
	ev.outVars.Assign("MAKEFILE_LIST", makefile_list)

	for _, stmt := range mk.stmts {
		ev.eval(stmt)
	}
	return &EvalResult{
		vars:     ev.outVars,
		rules:    ev.outRules,
		ruleVars: ev.outRuleVars,
	}, nil
}
