package main

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

type Rule struct {
	output string
	inputs []string
	cmds   []string
	filename string
	lineno int
	cmdLineno int
}

type EvalResult struct {
	vars  map[string]string
	rules []*Rule
	refs  map[string]bool
}

type Evaluator struct {
	outVars  map[string]string
	outRules []*Rule
	refs     map[string]bool
	vars     map[string]string
	curRule  *Rule

	funcs map[string]Func

	filename string
	lineno   int
}

func newEvaluator(vars map[string]string) *Evaluator {
	return &Evaluator{
		outVars: make(map[string]string),
		refs:    make(map[string]bool),
		vars:    vars,
		funcs: map[string]Func{
			"wildcard": funcWildcard,
			"shell":    funcShell,
			"warning":  funcWarning,
		},
	}
}

func (ev *Evaluator) evalFunction(ex string) (string, bool) {
	i := strings.IndexAny(ex, " \t")
	if i < 0 {
		return "", false
	}
	cmd := strings.TrimSpace(ex[:i])
	args := strings.TrimLeft(ex[i+1:], " \t")
	if f, ok := ev.funcs[cmd]; ok {
		return f(ev, args), true
	}
	return "", false
}

func (ev *Evaluator) evalExprSlice(ex string, term byte) (string, int) {
	var buf bytes.Buffer
	i := 0
	for i < len(ex) && ex[i] != term {
		ch := ex[i]
		i++
		switch ch {
		case '$':
			if i >= len(ex) || ex[i] == term {
				continue
			}

			var varname string
			switch ex[i] {
			case '@':
				buf.WriteString(ev.curRule.output)
				i++
				continue
			case '(':
				v, j := ev.evalExprSlice(ex[i+1:], ')')
				i += j + 2
				if r, done := ev.evalFunction(v); done {
					buf.WriteString(r)
					continue
				}

				varname = v
			default:
				varname = string(ex[i])
				i++
			}

			value, present := ev.vars[varname]
			if !present {
				ev.refs[varname] = true
				value = ev.outVars[varname]
			}
			buf.WriteString(ev.evalExpr(value))

		default:
			buf.WriteByte(ch)
		}
	}
	return buf.String(), i
}

func (ev *Evaluator) evalExpr(ex string) string {
	r, i := ev.evalExprSlice(ex, 0)
	if len(ex) != i {
		panic("Had a null character?")
	}
	return r
}

func (ev *Evaluator) evalAssign(ast *AssignAST) {
	ev.filename = ast.filename
	ev.lineno = ast.lineno

	lhs := ev.evalExpr(ast.lhs)
	rhs := ast.evalRHS(ev, lhs)
	Log("ASSIGN: %s=%s", lhs, rhs)
	ev.outVars[lhs] = rhs
}

func (ev *Evaluator) evalRule(ast *RuleAST) {
	ev.filename = ast.filename
	ev.lineno = ast.lineno

	ev.curRule = &Rule{
		filename: ast.filename,
		lineno: ast.lineno,
		cmdLineno: ast.cmdLineno,
	}
	lhs := ev.evalExpr(ast.lhs)
	ev.curRule.output = lhs
	rhs := strings.TrimSpace(ev.evalExpr(ast.rhs))
	if rhs != "" {
		re, err := regexp.Compile(`\s+`)
		if err != nil {
			panic(err)
		}
		ev.curRule.inputs = re.Split(rhs, -1)
	}
	var cmds []string
	for _, cmd := range ast.cmds {
		cmds = append(cmds, ev.evalExpr(cmd))
	}
	Log("RULE: %s=%s", lhs, rhs)
	ev.curRule.cmds = cmds
	ev.outRules = append(ev.outRules, ev.curRule)
	ev.curRule = nil
}

func (ev *Evaluator) evalRawExpr(ast *RawExprAST) {
	ev.filename = ast.filename
	ev.lineno = ast.lineno

	result := ev.evalExpr(ast.expr)
	if result != "" {
		// TODO: Fix rule_in_var.mk.
		Error(ast.filename, ast.lineno, "*** missing separator.")
	}
}

func (ev *Evaluator) getVar(name string) (string, bool) {
	value, present := ev.outVars[name]
	if present {
		return value, true
	}
	value, present = ev.vars[name]
	if present {
		return value, true
	}
	return "", false
}

func (ev *Evaluator) getVars() map[string]string {
	vars := make(map[string]string)
	for k, v := range ev.vars {
		vars[k] = v
	}
	for k, v := range ev.outVars {
		vars[k] = v
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

		er, err2 := Eval(mk, ev.getVars())
		if err2 != nil {
			panic(err2)
		}

		for k, v := range er.vars {
			ev.outVars[k] = v
		}
		for _, r := range er.rules {
			ev.outRules = append(ev.outRules, r)
		}
		for r, _ := range er.refs {
			ev.refs[r] = true
		}
	}
}

func (ev *Evaluator) evalIf(ast *IfAST) {
	var stmts []AST
	switch ast.op {
	case "ifdef", "ifndef":
		value, _ := ev.getVar(ev.evalExpr(ast.lhs))
		if (value != "") == (ast.op == "ifdef") {
			stmts = ast.trueStmts
		} else {
			stmts = ast.falseStmts
		}
	case "ifeq", "ifneq":
		panic("TODO")
	default:
		panic(fmt.Sprintf("unknown if statement: %q", ast.op))
	}
	for _, stmt := range stmts {
		ev.eval(stmt)
	}
}

func (ev *Evaluator) eval(ast AST) {
	ast.eval(ev)
}

func Eval(mk Makefile, vars map[string]string) (er *EvalResult, err error) {
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
		refs:  ev.refs,
	}, nil
}
