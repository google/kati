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
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/golang/glog"
)

type execContext struct {
	shell string

	mu     sync.Mutex
	ev     *Evaluator
	vpaths searchPaths
	output string
	inputs []string
}

func newExecContext(vars Vars, vpaths searchPaths, avoidIO bool) *execContext {
	ev := NewEvaluator(vars)
	ev.avoidIO = avoidIO

	ctx := &execContext{
		ev:     ev,
		vpaths: vpaths,
	}
	av := autoVar{ctx: ctx}
	for k, v := range map[string]Var{
		"@": autoAtVar{autoVar: av},
		"<": autoLessVar{autoVar: av},
		"^": autoHatVar{autoVar: av},
		"+": autoPlusVar{autoVar: av},
		"*": autoStarVar{autoVar: av},
	} {
		ev.vars[k] = v
		// $<k>D = $(patsubst %/,%,$(dir $<k>))
		ev.vars[k+"D"] = suffixDVar(k)
		// $<k>F = $(notdir $<k>)
		ev.vars[k+"F"] = suffixFVar(k)
	}

	// TODO: We should move this to somewhere around evalCmd so that
	// we can handle SHELL in target specific variables.
	shell, err := ev.EvaluateVar("SHELL")
	if err != nil {
		shell = "/bin/sh"
	}
	ctx.shell = shell
	return ctx
}

func (ec *execContext) uniqueInputs() []string {
	var uniqueInputs []string
	seen := make(map[string]bool)
	for _, input := range ec.inputs {
		if !seen[input] {
			seen[input] = true
			uniqueInputs = append(uniqueInputs, input)
		}
	}
	return uniqueInputs
}

type autoVar struct{ ctx *execContext }

func (v autoVar) Flavor() string  { return "undefined" }
func (v autoVar) Origin() string  { return "automatic" }
func (v autoVar) IsDefined() bool { return true }
func (v autoVar) Append(*Evaluator, string) (Var, error) {
	return nil, fmt.Errorf("cannot append to autovar")
}
func (v autoVar) AppendVar(*Evaluator, Value) (Var, error) {
	return nil, fmt.Errorf("cannot append to autovar")
}
func (v autoVar) serialize() serializableVar {
	return serializableVar{Type: ""}
}
func (v autoVar) dump(d *dumpbuf) {
	d.err = fmt.Errorf("cannot dump auto var: %v", v)
}

type autoAtVar struct{ autoVar }

func (v autoAtVar) Eval(w evalWriter, ev *Evaluator) error {
	fmt.Fprint(w, v.String())
	return nil
}
func (v autoAtVar) String() string { return v.ctx.output }

type autoLessVar struct{ autoVar }

func (v autoLessVar) Eval(w evalWriter, ev *Evaluator) error {
	fmt.Fprint(w, v.String())
	return nil
}
func (v autoLessVar) String() string {
	if len(v.ctx.inputs) > 0 {
		return v.ctx.inputs[0]
	}
	return ""
}

type autoHatVar struct{ autoVar }

func (v autoHatVar) Eval(w evalWriter, ev *Evaluator) error {
	fmt.Fprint(w, v.String())
	return nil
}
func (v autoHatVar) String() string {
	return strings.Join(v.ctx.uniqueInputs(), " ")
}

type autoPlusVar struct{ autoVar }

func (v autoPlusVar) Eval(w evalWriter, ev *Evaluator) error {
	fmt.Fprint(w, v.String())
	return nil
}
func (v autoPlusVar) String() string { return strings.Join(v.ctx.inputs, " ") }

type autoStarVar struct{ autoVar }

func (v autoStarVar) Eval(w evalWriter, ev *Evaluator) error {
	fmt.Fprint(w, v.String())
	return nil
}

// TODO: Use currentStem. See auto_stem_var.mk
func (v autoStarVar) String() string { return stripExt(v.ctx.output) }

func suffixDVar(k string) Var {
	return &recursiveVar{
		expr: expr{
			&funcPatsubst{
				fclosure: fclosure{
					args: []Value{
						literal("(patsubst"),
						literal("%/"),
						literal("%"),
						&funcDir{
							fclosure: fclosure{
								args: []Value{
									literal("(dir"),
									&varref{
										varname: literal(k),
									},
								},
							},
						},
					},
				},
			},
		},
		origin: "automatic",
	}
}

func suffixFVar(k string) Var {
	return &recursiveVar{
		expr: expr{
			&funcNotdir{
				fclosure: fclosure{
					args: []Value{
						literal("(notdir"),
						&varref{varname: literal(k)},
					},
				},
			},
		},
		origin: "automatic",
	}
}

// runner is a single shell command invocation.
type runner struct {
	output      string
	cmd         string
	echo        bool
	ignoreError bool
	shell       string
}

func (r runner) String() string {
	cmd := r.cmd
	if !r.echo {
		cmd = "@" + cmd
	}
	if r.ignoreError {
		cmd = "-" + cmd
	}
	return cmd
}

func (r runner) forCmd(s string) runner {
	for {
		s = trimLeftSpace(s)
		if s == "" {
			return runner{}
		}
		switch s[0] {
		case '@':
			if !DryRunFlag {
				r.echo = false
			}
			s = s[1:]
			continue
		case '-':
			r.ignoreError = true
			s = s[1:]
			continue
		}
		break
	}
	r.cmd = s
	return r
}

func (r runner) eval(ev *Evaluator, s string) ([]runner, error) {
	r = r.forCmd(s)
	if strings.IndexByte(r.cmd, '$') < 0 {
		// fast path
		return []runner{r}, nil
	}
	// TODO(ukai): parse once more earlier?
	expr, _, err := parseExpr([]byte(r.cmd), nil, parseOp{})
	if err != nil {
		return nil, ev.errorf("parse cmd %q: %v", r.cmd, err)
	}
	buf := newEbuf()
	err = expr.Eval(buf, ev)
	if err != nil {
		return nil, err
	}
	cmds := buf.String()
	buf.release()
	glog.V(1).Infof("evalcmd: %q => %q", r.cmd, cmds)
	var runners []runner
	for _, cmd := range strings.Split(cmds, "\n") {
		if len(runners) > 0 && strings.HasSuffix(runners[len(runners)-1].cmd, "\\") {
			runners[len(runners)-1].cmd += "\n"
			runners[len(runners)-1].cmd += cmd
			continue
		}
		runners = append(runners, r.forCmd(cmd))
	}
	return runners, nil
}

func (r runner) run(output string) error {
	if r.echo || DryRunFlag {
		fmt.Printf("%s\n", r.cmd)
	}
	s := cmdline(r.cmd)
	glog.Infof("sh:%q", s)
	if DryRunFlag {
		return nil
	}
	args := []string{r.shell, "-c", s}
	cmd := exec.Cmd{
		Path: args[0],
		Args: args,
	}
	out, err := cmd.CombinedOutput()
	fmt.Printf("%s", out)
	exit := exitStatus(err)
	if r.ignoreError && exit != 0 {
		fmt.Printf("[%s] Error %d (ignored)\n", output, exit)
		err = nil
	}
	return err
}

func createRunners(ctx *execContext, n *DepNode) ([]runner, bool, error) {
	var runners []runner
	if len(n.Cmds) == 0 {
		return runners, false, nil
	}

	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	// For automatic variables.
	ctx.output = n.Output
	ctx.inputs = n.ActualInputs
	for k, v := range n.TargetSpecificVars {
		restore := ctx.ev.vars.save(k)
		defer restore()
		ctx.ev.vars[k] = v
		if glog.V(1) {
			glog.Infof("set tsv: %s=%s", k, v)
		}
	}

	ctx.ev.filename = n.Filename
	ctx.ev.lineno = n.Lineno
	glog.Infof("Building: %s cmds:%q", n.Output, n.Cmds)
	r := runner{
		output: n.Output,
		echo:   true,
		shell:  ctx.shell,
	}
	for _, cmd := range n.Cmds {
		rr, err := r.eval(ctx.ev, cmd)
		if err != nil {
			return nil, false, err
		}
		for _, r := range rr {
			if len(r.cmd) != 0 {
				runners = append(runners, r)
			}
		}
	}
	if len(ctx.ev.delayedOutputs) > 0 {
		var nrunners []runner
		r := runner{
			output: n.Output,
			shell:  ctx.shell,
		}
		for _, o := range ctx.ev.delayedOutputs {
			nrunners = append(nrunners, r.forCmd(o))
		}
		nrunners = append(nrunners, runners...)
		runners = nrunners
		ctx.ev.delayedOutputs = nil
	}
	return runners, ctx.ev.hasIO, nil
}

func evalCommands(nodes []*DepNode, vars Vars) error {
	ioCnt := 0
	ectx := newExecContext(vars, searchPaths{}, true)
	for i, n := range nodes {
		runners, hasIO, err := createRunners(ectx, n)
		if err != nil {
			return err
		}
		if hasIO {
			ioCnt++
			if ioCnt%100 == 0 {
				logStats("%d/%d rules have IO", ioCnt, i+1)
			}
			continue
		}

		n.Cmds = []string{}
		n.TargetSpecificVars = make(Vars)
		for _, r := range runners {
			n.Cmds = append(n.Cmds, r.String())
		}
	}
	logStats("%d/%d rules have IO", ioCnt, len(nodes))
	return nil
}
