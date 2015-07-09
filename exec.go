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
	"os"
	"time"
)

// Executor manages execution of makefile rules.
type Executor struct {
	rules         map[string]*rule
	implicitRules []*rule
	suffixRules   map[string][]*rule
	firstRule     *rule
	// target -> Job, nil means the target is currently being processed.
	done map[string]*job

	wm *workerManager

	ctx *execContext

	trace          []string
	buildCnt       int
	alreadyDoneCnt int
	noRuleCnt      int
	upToDateCnt    int
	runCommandCnt  int
}

func (ex *Executor) makeJobs(n *DepNode, neededBy *job) error {
	output, _ := existsInVPATH(ex.ctx.ev, n.Output)
	if neededBy != nil {
		logf("MakeJob: %s for %s", output, neededBy.n.Output)
	}
	n.Output = output
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
		numDeps: len(n.Deps) + len(n.OrderOnlys),
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
		deps = append(deps, d)
	}
	for _, d := range n.OrderOnlys {
		// need lock for ex.ctx.ev?
		if _, ok := existsInVPATH(ex.ctx.ev, d.Output); ok {
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
	NumJobs int
}

// NewExecutor creates new Executor.
func NewExecutor(opt *ExecutorOpt) (*Executor, error) {
	if opt == nil {
		opt = &ExecutorOpt{NumJobs: 1}
	}
	if opt.NumJobs < 1 {
		opt.NumJobs = 1
	}
	wm, err := newWorkerManager(opt.NumJobs)
	if err != nil {
		return nil, err
	}
	ex := &Executor{
		rules:       make(map[string]*rule),
		suffixRules: make(map[string][]*rule),
		done:        make(map[string]*job),
		wm:          wm,
	}
	return ex, nil
}

// Exec executes to build roots.
func (ex *Executor) Exec(g *DepGraph) error {
	ex.ctx = newExecContext(g.vars, false)

	// TODO: Handle target specific variables.
	for name, export := range g.exports {
		if export {
			v, err := ex.ctx.ev.EvaluateVar(name)
			if err != nil {
				return err
			}
			os.Setenv(name, v)
		} else {
			os.Unsetenv(name)
		}
	}

	startTime := time.Now()
	for _, root := range g.nodes {
		err := ex.makeJobs(root, nil)
		if err != nil {
			break
		}
	}
	err := ex.wm.Wait()
	logStats("exec time: %q", time.Since(startTime))
	return err
}
