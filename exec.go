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
	"time"
)

// Executor manages execution of makefile rules.
type Executor struct {
	rules         map[string]*rule
	implicitRules []*rule
	suffixRules   map[string][]*rule
	firstRule     *rule
	shell         string
	vars          Vars
	varsLock      sync.Mutex
	// target -> Job, nil means the target is currently being processed.
	done map[string]*job

	wm *workerManager

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

func (v autoAtVar) Eval(w io.Writer, ev *Evaluator) error {
	fmt.Fprint(w, v.ex.currentOutput)
	return nil
}
func (v autoAtVar) String() string { return "$*" }

type autoLessVar struct{ autoVar }

func (v autoLessVar) Eval(w io.Writer, ev *Evaluator) error {
	if len(v.ex.currentInputs) > 0 {
		fmt.Fprint(w, v.ex.currentInputs[0])
	}
	return nil
}
func (v autoLessVar) String() string { return "$<" }

type autoHatVar struct{ autoVar }

func (v autoHatVar) Eval(w io.Writer, ev *Evaluator) error {
	var uniqueInputs []string
	seen := make(map[string]bool)
	for _, input := range v.ex.currentInputs {
		if !seen[input] {
			seen[input] = true
			uniqueInputs = append(uniqueInputs, input)
		}
	}
	fmt.Fprint(w, strings.Join(uniqueInputs, " "))
	return nil
}
func (v autoHatVar) String() string { return "$^" }

type autoPlusVar struct{ autoVar }

func (v autoPlusVar) Eval(w io.Writer, ev *Evaluator) error {
	fmt.Fprint(w, strings.Join(v.ex.currentInputs, " "))
	return nil
}
func (v autoPlusVar) String() string { return "$+" }

type autoStarVar struct{ autoVar }

func (v autoStarVar) Eval(w io.Writer, ev *Evaluator) error {
	// TODO: Use currentStem. See auto_stem_var.mk
	fmt.Fprint(w, stripExt(v.ex.currentOutput))
	return nil
}
func (v autoStarVar) String() string { return "$*" }

type autoSuffixDVar struct {
	autoVar
	v Var
}

func (v autoSuffixDVar) Eval(w io.Writer, ev *Evaluator) error {
	var buf bytes.Buffer
	err := v.v.Eval(&buf, ev)
	if err != nil {
		return err
	}
	ws := newWordScanner(buf.Bytes())
	sw := ssvWriter{w: w}
	for ws.Scan() {
		sw.WriteString(filepath.Dir(string(ws.Bytes())))
	}
	return nil
}

func (v autoSuffixDVar) String() string { return v.v.String() + "D" }

type autoSuffixFVar struct {
	autoVar
	v Var
}

func (v autoSuffixFVar) Eval(w io.Writer, ev *Evaluator) error {
	var buf bytes.Buffer
	err := v.v.Eval(&buf, ev)
	if err != nil {
		return err
	}
	ws := newWordScanner(buf.Bytes())
	sw := ssvWriter{w: w}
	for ws.Scan() {
		sw.WriteString(filepath.Base(string(ws.Bytes())))
	}
	return nil
}

func (v autoSuffixFVar) String() string { return v.v.String() + "F" }

func (ex *Executor) makeJobs(n *DepNode, neededBy *job) error {
	output := n.Output
	if neededBy != nil {
		logf("MakeJob: %s for %s", output, neededBy.n.Output)
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
			logf("%s already done: %d", j.n.Output, j.outputTs)
			if neededBy != nil {
				ex.wm.ReportNewDep(j, neededBy)
			}
		}
		return nil
	}

	j = &job{
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
	logf("new: %s (%d)", j.n.Output, j.numDeps)

	for _, d := range deps {
		ex.trace = append(ex.trace, d.Output)
		err := ex.makeJobs(d, j)
		ex.trace = ex.trace[0 : len(ex.trace)-1]
		if err != nil {
			return err
		}
	}

	ex.done[output] = j
	return ex.wm.PostJob(j)
}

func (ex *Executor) reportStats() {
	if !LogFlag && !PeriodicStatsFlag {
		return
	}

	logStats("build=%d alreadyDone=%d noRule=%d, upToDate=%d runCommand=%d",
		ex.buildCnt, ex.alreadyDoneCnt, ex.noRuleCnt, ex.upToDateCnt, ex.runCommandCnt)
	if len(ex.trace) > 1 {
		logStats("trace=%q", ex.trace)
	}
}

// ExecutorOpt is an option for Executor.
type ExecutorOpt struct {
	NumJobs  int
	ParaPath string
}

// NewExecutor creates new Executor.
func NewExecutor(vars Vars, opt *ExecutorOpt) (*Executor, error) {
	if opt == nil {
		opt = &ExecutorOpt{NumJobs: 1}
	}
	if opt.NumJobs < 1 {
		opt.NumJobs = 1
	}
	wm, err := newWorkerManager(opt.NumJobs, opt.ParaPath)
	if err != nil {
		return nil, err
	}
	ex := &Executor{
		rules:       make(map[string]*rule),
		suffixRules: make(map[string][]*rule),
		done:        make(map[string]*job),
		vars:        vars,
		wm:          wm,
	}
	// TODO: We should move this to somewhere around evalCmd so that
	// we can handle SHELL in target specific variables.
	ev := NewEvaluator(ex.vars)
	ex.shell, err = ev.EvaluateVar("SHELL")
	if err != nil {
		ex.shell = "/bin/sh"
	}
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
	return ex, nil
}

// Exec executes to build roots.
func (ex *Executor) Exec(roots []*DepNode) error {
	startTime := time.Now()
	for _, root := range roots {
		err := ex.makeJobs(root, nil)
		if err != nil {
			break
		}
	}
	err := ex.wm.Wait()
	logStats("exec time: %q", time.Since(startTime))
	return err
}

func (ex *Executor) createRunners(n *DepNode, avoidIO bool) ([]runner, bool, error) {
	var runners []runner
	if len(n.Cmds) == 0 {
		return runners, false, nil
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
		logf("tsv: %s=%s", k, v)
	}

	ev := NewEvaluator(ex.vars)
	ev.avoidIO = avoidIO
	ev.filename = n.Filename
	ev.lineno = n.Lineno
	logf("Building: %s cmds:%q", n.Output, n.Cmds)
	r := runner{
		output: n.Output,
		echo:   true,
		shell:  ex.shell,
	}
	for _, cmd := range n.Cmds {
		rr, err := evalCmd(ev, r, cmd)
		if err != nil {
			return nil, false, err
		}
		for _, r := range rr {
			if len(r.cmd) != 0 {
				runners = append(runners, r)
			}
		}
	}
	return runners, ev.hasIO, nil
}

func evalCommands(nodes []*DepNode, vars Vars) error {
	ioCnt := 0
	ex, err := NewExecutor(vars, nil)
	if err != nil {
		return err
	}
	for i, n := range nodes {
		runners, hasIO, err := ex.createRunners(n, true)
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

	err = ex.wm.Wait()
	logStats("%d/%d rules have IO", ioCnt, len(nodes))
	return err
}
