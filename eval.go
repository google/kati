package main

import (
	"bytes"
	"fmt"
	"strings"
)

type EvalResult struct {
	vars  *VarTab
	rules []*Rule
}

type Evaluator struct {
	outVars  *VarTab
	outRules []*Rule
	vars     *VarTab
	curRule  *Rule

	funcs map[string]Func

	filename string
	lineno   int
}

func newEvaluator(vars *VarTab) *Evaluator {
	return &Evaluator{
		outVars: NewVarTab(nil),
		vars:    vars,
		funcs: map[string]Func{
			"subst":    funcSubst,
			"patsubst": funcPatsubst,
			"wildcard": funcWildcard,
			"realpath": funcRealpath,
			"abspath":  funcAbspath,
			"shell":    funcShell,
			"flavor":   funcFlavor,
			"warning":  funcWarning,
		},
	}
}

func (ev *Evaluator) evalFunction(args []string) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	i := strings.IndexAny(args[0], " \t")
	if i < 0 {
		return "", false
	}
	cmd := strings.TrimSpace(args[0][:i])
	args[0] = strings.TrimLeft(args[0][i+1:], " \t")
	if f, ok := ev.funcs[cmd]; ok {
		return f(ev, args), true
	}
	return "", false
}

func (ev *Evaluator) evalExprSlice(ex string) (string, int) {
	var buf bytes.Buffer
	i := 0
Loop:
	for i < len(ex) {
		ch := ex[i]
		i++
		switch ch {
		case '$':
			if i >= len(ex) {
				break Loop
			}

			var varname string
			switch ex[i] {
			case '$':
				buf.WriteByte('$')
				i++
				continue
			case '(', '{':
				args, rest, err := parseExpr(ex[i:])
				if err != nil {
				}
				i += rest
				if r, done := ev.evalFunction(args); done {
					buf.WriteString(r)
					continue
				}

				varname = strings.Join(args, ",")
				varname = ev.evalExpr(varname)
			default:
				varname = string(ex[i])
				i++
			}

			// TODO: lookup order is correct?
			value := ev.vars.Lookup(varname)
			if !value.IsDefined() {
				value = ev.outVars.Lookup(varname)
			}
			Log("var %q=>%q=>%q", varname, value, value.Eval(ev))
			buf.WriteString(value.Eval(ev))

		default:
			buf.WriteByte(ch)
		}
	}
	return buf.String(), i
}

func (ev *Evaluator) evalExpr(ex string) string {
	r, i := ev.evalExprSlice(ex)
	if len(ex) != i {
		panic(fmt.Sprintf("Had a null character? %q, %d", ex, i))
	}
	return r
}

func (ev *Evaluator) evalAssign(ast *AssignAST) {
	ev.filename = ast.filename
	ev.lineno = ast.lineno

	lhs := ev.evalExpr(ast.lhs)
	rhs := ast.evalRHS(ev, lhs)
	Log("ASSIGN: %s=%q (flavor:%q)", lhs, rhs, rhs.Flavor())
	ev.outVars.Assign(lhs, rhs)
}

func (ev *Evaluator) evalMaybeRule(ast *MaybeRuleAST) {
	ev.filename = ast.filename
	ev.lineno = ast.lineno

	line := ev.evalExpr(ast.expr)
	Log("rule? %q=>%q", ast.expr, line)

	if strings.TrimSpace(line) == "" {
		if len(ast.cmds) > 0 {
			Error(ast.filename, ast.cmdLineno, "*** commands commence before first target.")
		}
		return
	}

	ev.curRule = &Rule{
		filename:  ast.filename,
		lineno:    ast.lineno,
		cmdLineno: ast.cmdLineno,
	}
	if err := ev.curRule.parse(line); err != "" {
		Error(ast.filename, ast.lineno, err)
	}
	Log("rule %q => outputs:%q, inputs:%q", line, ev.curRule.outputs, ev.curRule.inputs)
	// It seems rules with no outputs are siliently ignored.
	if len(ev.curRule.outputs) == 0 && len(ev.curRule.outputPatterns) == 0 {
		ev.curRule = nil
		return
	}

	// TODO: Pretty print.
	//Log("RULE: %s=%s (%d commands)", lhs, rhs, len(cmds))

	ev.curRule.cmds = ast.cmds
	ev.outRules = append(ev.outRules, ev.curRule)
	ev.curRule = nil
}

func (ev *Evaluator) LookupVar(name string) Var {
	v := ev.outVars.Lookup(name)
	if v.IsDefined() {
		return v
	}
	return ev.vars.Lookup(name)
}

func (ev *Evaluator) VarTab() *VarTab {
	vars := NewVarTab(nil)
	for k, v := range ev.vars.Vars() {
		vars.Assign(k, v)
	}
	for k, v := range ev.outVars.Vars() {
		vars.Assign(k, v)
	}
	return vars
}

func (ev *Evaluator) evalInclude(ast *IncludeAST) {
	ev.filename = ast.filename
	ev.lineno = ast.lineno

	// TODO: Handle glob
	files := strings.Split(ev.evalExpr(ast.expr), " ")
	for _, file := range files {
		mk, err := ParseMakefile(file)
		if err != nil {
			if ast.op == "include" {
				panic(err)
			} else {
				continue
			}
		}

		er, err2 := Eval(mk, ev.VarTab())
		if err2 != nil {
			panic(err2)
		}

		for k, v := range er.vars.Vars() {
			ev.outVars.Assign(k, v)
		}
		for _, r := range er.rules {
			ev.outRules = append(ev.outRules, r)
		}
	}
}

func (ev *Evaluator) evalIf(ast *IfAST) {
	var isTrue bool
	switch ast.op {
	case "ifdef", "ifndef":
		value := ev.LookupVar(ev.evalExpr(ast.lhs)).Eval(ev)
		isTrue = value != "" == (ast.op == "ifdef")
		Log("%s lhs=%q value=%q => %t", ast.op, ast.lhs, value, isTrue)
	case "ifeq", "ifneq":
		lhs := ev.evalExpr(ast.lhs)
		rhs := ev.evalExpr(ast.rhs)
		isTrue = (lhs == rhs) == (ast.op == "ifeq")
		Log("%s lhs=%q %q rhs=%q %q => %t", ast.op, ast.lhs, lhs, ast.rhs, rhs)
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

func Eval(mk Makefile, vars *VarTab) (er *EvalResult, err error) {
	ev := newEvaluator(vars)
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	for _, stmt := range mk.stmts {
		ev.eval(stmt)
	}
	return &EvalResult{
		vars:  ev.outVars,
		rules: ev.outRules,
	}, nil
}
