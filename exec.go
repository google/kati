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
	"io"
	"path/filepath"
	"strings"
	"sync"
)

type Executor struct {
	rules         map[string]*Rule
	implicitRules []*Rule
	suffixRules   map[string][]*Rule
	firstRule     *Rule
	shell         string
	vars          Vars
	varsLock      sync.Mutex
	// target -> Job, nil means the target is currently being processed.
	done map[string]*Job

	wm *WorkerManager

	currentOutput string
	currentInputs []string
	currentStem   string

	trace          []string
	buildCnt       int
	alreadyDoneCnt int
	noRuleCnt      int
	upToDateCnt    int
	runCommandCnt  int
}

type autoVar struct{ ex *Executor }

func (v autoVar) Flavor() string  { return "undefined" }
func (v autoVar) Origin() string  { return "automatic" }
func (v autoVar) IsDefined() bool { panic("not implemented") }
func (v autoVar) String() string  { panic("not implemented") }
func (v autoVar) Append(*Evaluator, string) Var {
	panic("must not be called")
}
func (v autoVar) AppendVar(*Evaluator, Value) Var {
	panic("must not be called")
}
func (v autoVar) Serialize() SerializableVar {
	panic(fmt.Sprintf("cannot serialize auto var: %q", v))
}
func (v autoVar) Dump(w io.Writer) {
	panic(fmt.Sprintf("cannot dump auto var: %q", v))
}

type autoAtVar struct{ autoVar }

func (v autoAtVar) Eval(w io.Writer, ev *Evaluator) {
	fmt.Fprint(w, v.ex.currentOutput)
}

type autoLessVar struct{ autoVar }

func (v autoLessVar) Eval(w io.Writer, ev *Evaluator) {
	if len(v.ex.currentInputs) > 0 {
		fmt.Fprint(w, v.ex.currentInputs[0])
	}
}

type autoHatVar struct{ autoVar }

func (v autoHatVar) Eval(w io.Writer, ev *Evaluator) {
	var uniqueInputs []string
	seen := make(map[string]bool)
	for _, input := range v.ex.currentInputs {
		if !seen[input] {
			seen[input] = true
			uniqueInputs = append(uniqueInputs, input)
		}
	}
	fmt.Fprint(w, strings.Join(uniqueInputs, " "))
}

type autoPlusVar struct{ autoVar }

func (v autoPlusVar) Eval(w io.Writer, ev *Evaluator) {
	fmt.Fprint(w, strings.Join(v.ex.currentInputs, " "))
}

type autoStarVar struct{ autoVar }

func (v autoStarVar) Eval(w io.Writer, ev *Evaluator) {
	// TODO: Use currentStem. See auto_stem_var.mk
	fmt.Fprint(w, stripExt(v.ex.currentOutput))
}

type autoSuffixDVar struct {
	autoVar
	v Var
}

func (v autoSuffixDVar) Eval(w io.Writer, ev *Evaluator) {
	var buf bytes.Buffer
	v.v.Eval(&buf, ev)
	ws := newWordScanner(buf.Bytes())
	sw := ssvWriter{w: w}
	for ws.Scan() {
		sw.WriteString(filepath.Dir(string(ws.Bytes())))
	}
}

type autoSuffixFVar struct {
	autoVar
	v Var
}

func (v autoSuffixFVar) Eval(w io.Writer, ev *Evaluator) {
	var buf bytes.Buffer
	v.v.Eval(&buf, ev)
	ws := newWordScanner(buf.Bytes())
	sw := ssvWriter{w: w}
	for ws.Scan() {
		sw.WriteString(filepath.Base(string(ws.Bytes())))
	}
}

func (ex *Executor) makeJobs(n *DepNode, neededBy *Job) error {
	output := n.Output
	if neededBy != nil {
		Logf("MakeJob: %s for %s", output, neededBy.n.Output)
	}
	ex.buildCnt++
	if ex.buildCnt%100 == 0 {
		ex.reportStats()
	}

	j, present := ex.done[output]

	if present {
		if j == nil {
			if !n.IsPhony {
				fmt.Printf("Circular %s <- %s dependency dropped.\n", neededBy.n.Output, n.Output)
			}
			if neededBy != nil {
				neededBy.numDeps--
			}
		} else {
			Logf("%s already done: %d", j.n.Output, j.outputTs)
			if neededBy != nil {
				ex.wm.ReportNewDep(j, neededBy)
			}
		}
		return nil
	}

	j = &Job{
		n:       n,
		ex:      ex,
		numDeps: len(n.Deps),
		depsTs:  int64(-1),
	}
	if neededBy != nil {
		j.parents = append(j.parents, neededBy)
	}

	ex.done[output] = nil
	// We iterate n.Deps twice. In the first run, we may modify
	// numDeps. There will be a race if we do so after the first
	// ex.makeJobs(d, j).
	var deps []*DepNode
	for _, d := range n.Deps {
		if d.IsOrderOnly && exists(d.Output) {
			j.numDeps--
			continue
		}
		deps = append(deps, d)
	}
	Logf("new: %s (%d)", j.n.Output, j.numDeps)

	for _, d := range deps {
		ex.trace = append(ex.trace, d.Output)
		err := ex.makeJobs(d, j)
		ex.trace = ex.trace[0 : len(ex.trace)-1]
		if err != nil {
			return err
		}
	}

	ex.done[output] = j
	ex.wm.PostJob(j)

	return nil
}

func (ex *Executor) reportStats() {
	if !LogFlag && !PeriodicStatsFlag {
		return
	}

	LogStats("build=%d alreadyDone=%d noRule=%d, upToDate=%d runCommand=%d",
		ex.buildCnt, ex.alreadyDoneCnt, ex.noRuleCnt, ex.upToDateCnt, ex.runCommandCnt)
	if len(ex.trace) > 1 {
		LogStats("trace=%q", ex.trace)
	}
}

type ExecutorOpt struct {
	NumJobs  int
	ParaPath string
}

func NewExecutor(vars Vars, opt *ExecutorOpt) *Executor {
	if opt == nil {
		opt = &ExecutorOpt{NumJobs: 1}
	}
	if opt.NumJobs < 1 {
		opt.NumJobs = 1
	}
	ex := &Executor{
		rules:       make(map[string]*Rule),
		suffixRules: make(map[string][]*Rule),
		done:        make(map[string]*Job),
		vars:        vars,
		wm:          NewWorkerManager(opt.NumJobs, opt.ParaPath),
	}
	// TODO: We should move this to somewhere around evalCmd so that
	// we can handle SHELL in target specific variables.
	ev := NewEvaluator(ex.vars)
	ex.shell = ev.EvaluateVar("SHELL")
	for k, v := range map[string]Var{
		"@": autoAtVar{autoVar: autoVar{ex: ex}},
		"<": autoLessVar{autoVar: autoVar{ex: ex}},
		"^": autoHatVar{autoVar: autoVar{ex: ex}},
		"+": autoPlusVar{autoVar: autoVar{ex: ex}},
		"*": autoStarVar{autoVar: autoVar{ex: ex}},
	} {
		ex.vars[k] = v
		ex.vars[k+"D"] = autoSuffixDVar{v: v}
		ex.vars[k+"F"] = autoSuffixFVar{v: v}
	}
	return ex
}

func (ex *Executor) Exec(roots []*DepNode) error {
	for _, root := range roots {
		ex.makeJobs(root, nil)
	}
	ex.wm.Wait()
	return nil
}

func (ex *Executor) createRunners(n *DepNode, avoidIO bool) ([]runner, bool) {
	var runners []runner
	if len(n.Cmds) == 0 {
		return runners, false
	}

	var restores []func()
	defer func() {
		for i := len(restores) - 1; i >= 0; i-- {
			restores[i]()
		}
	}()

	ex.varsLock.Lock()
	restores = append(restores, func() { ex.varsLock.Unlock() })
	// For automatic variables.
	ex.currentOutput = n.Output
	ex.currentInputs = n.ActualInputs
	for k, v := range n.TargetSpecificVars {
		restores = append(restores, ex.vars.save(k))
		ex.vars[k] = v
	}

	ev := NewEvaluator(ex.vars)
	ev.avoidIO = avoidIO
	ev.filename = n.Filename
	ev.lineno = n.Lineno
	Logf("Building: %s cmds:%q", n.Output, n.Cmds)
	r := runner{
		output: n.Output,
		echo:   true,
		shell:  ex.shell,
	}
	for _, cmd := range n.Cmds {
		for _, r := range evalCmd(ev, r, cmd) {
			if len(r.cmd) != 0 {
				runners = append(runners, r)
			}
		}
	}
	return runners, ev.hasIO
}

func EvalCommands(nodes []*DepNode, vars Vars) {
	ioCnt := 0
	ex := NewExecutor(vars, nil)
	for i, n := range nodes {
		runners, hasIO := ex.createRunners(n, true)
		if hasIO {
			ioCnt++
			if ioCnt%100 == 0 {
				LogStats("%d/%d rules have IO", ioCnt, i+1)
			}
			continue
		}

		n.Cmds = []string{}
		n.TargetSpecificVars = make(Vars)
		for _, r := range runners {
			cmd := r.cmd
			// TODO: Do not preserve the effect of dryRunFlag.
			if r.echo {
				cmd = "@" + cmd
			}
			if r.ignoreError {
				cmd = "-" + cmd
			}
			n.Cmds = append(n.Cmds, cmd)
		}
	}

	LogStats("%d/%d rules have IO", ioCnt, len(nodes))
}
