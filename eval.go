package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type Rule struct {
	output string
	inputs []string
	cmds   []string
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
}

func newEvaluator() *Evaluator {
	return &Evaluator{
		outVars: make(map[string]string),
		refs:    make(map[string]bool),
		vars:    make(map[string]string),
	}
}

func (ev *Evaluator) evalFunction(ex string) (string, bool) {
	if strings.HasPrefix(ex, "wildcard ") {
		arg := ex[len("wildcard "):]

		files, err := filepath.Glob(arg)
		if err != nil {
			panic(err)
		}
		return strings.Join(files, " "), true
	} else if strings.HasPrefix(ex, "shell ") {
		arg := ex[len("shell "):]

		args := []string{"/bin/sh", "-c", arg}
		cmd := exec.Cmd{
			Path: args[0],
			Args: args,
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			panic(err)
		}
		re, err := regexp.Compile(`\s`)
		if err != nil {
			panic(err)
		}
		return string(re.ReplaceAllString(string(out), " ")), true
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
			buf.WriteString(value)

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
	lhs := ev.evalExpr(ast.lhs)
	rhs := ev.evalExpr(ast.rhs)
	Log("ASSIGN: %s=%s", lhs, rhs)
	ev.outVars[lhs] = rhs
}

func (ev *Evaluator) evalRule(ast *RuleAST) {
	ev.curRule = &Rule{}
	lhs := ev.evalExpr(ast.lhs)
	ev.curRule.output = lhs
	rhs := ev.evalExpr(ast.rhs)
	if rhs != "" {
		ev.curRule.inputs = strings.Split(rhs, " ")
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

func (ev *Evaluator) eval(ast AST) {
	ast.eval(ev)
}

func Eval(mk Makefile) *EvalResult {
	ev := newEvaluator()
	for _, stmt := range mk.stmts {
		ev.eval(stmt)
	}
	return &EvalResult{
		vars:  ev.outVars,
		rules: ev.outRules,
		refs:  ev.refs,
	}
}
