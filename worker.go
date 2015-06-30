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
	"container/heap"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

type job struct {
	n        *DepNode
	ex       *Executor
	parents  []*job
	outputTs int64
	numDeps  int
	depsTs   int64
	id       int

	runners []runner
}

type jobResult struct {
	j   *job
	w   *worker
	err error
}

type newDep struct {
	j        *job
	neededBy *job
}

type worker struct {
	wm       *workerManager
	jobChan  chan *job
	waitChan chan bool
	doneChan chan bool
}

type jobQueue []*job

func (jq jobQueue) Len() int      { return len(jq) }
func (jq jobQueue) Swap(i, j int) { jq[i], jq[j] = jq[j], jq[i] }

func (jq jobQueue) Less(i, j int) bool {
	// First come, first serve, for GNU make compatibility.
	return jq[i].id < jq[j].id
}

func (jq *jobQueue) Push(x interface{}) {
	item := x.(*job)
	*jq = append(*jq, item)
}

func (jq *jobQueue) Pop() interface{} {
	old := *jq
	n := len(old)
	item := old[n-1]
	*jq = old[0 : n-1]
	return item
}

func newWorker(wm *workerManager) *worker {
	w := &worker{
		wm:       wm,
		jobChan:  make(chan *job),
		waitChan: make(chan bool),
		doneChan: make(chan bool),
	}
	return w
}

func (w *worker) Run() {
	done := false
	for !done {
		select {
		case j := <-w.jobChan:
			err := j.build()
			w.wm.ReportResult(w, j, err)
		case done = <-w.waitChan:
		}
	}
	w.doneChan <- true
}

func (w *worker) PostJob(j *job) {
	w.jobChan <- j
}

func (w *worker) Wait() {
	w.waitChan <- true
	<-w.doneChan
}

func (j *job) createRunners() ([]runner, error) {
	runners, _, err := createRunners(j.ex.ctx, j.n)
	return runners, err
}

// TODO(ukai): use time.Time?
func getTimestamp(filename string) int64 {
	st, err := os.Stat(filename)
	if err != nil {
		return -2
	}
	return st.ModTime().Unix()
}

func (j *job) build() error {
	if j.n.IsPhony {
		j.outputTs = -2 // trigger cmd even if all inputs don't exist.
	} else {
		j.outputTs = getTimestamp(j.n.Output)
	}

	if !j.n.HasRule {
		if j.outputTs >= 0 || j.n.IsPhony {
			return nil
		}
		if len(j.parents) == 0 {
			return fmt.Errorf("*** No rule to make target %q.", j.n.Output)
		}
		return fmt.Errorf("*** No rule to make target %q, needed by %q.", j.n.Output, j.parents[0].n.Output)
	}

	if j.outputTs >= j.depsTs {
		// TODO: stats.
		return nil
	}

	rr, err := j.createRunners()
	if err != nil {
		return err
	}
	for _, r := range rr {
		err := r.run(j.n.Output)
		if err != nil {
			exit := exitStatus(err)
			return fmt.Errorf("[%s] Error %d: %v", j.n.Output, exit, err)
		}
	}

	if j.n.IsPhony {
		j.outputTs = time.Now().Unix()
	} else {
		j.outputTs = getTimestamp(j.n.Output)
		if j.outputTs < 0 {
			j.outputTs = time.Now().Unix()
		}
	}
	return nil
}

func (wm *workerManager) handleJobs() error {
	for {
		if wm.para == nil && len(wm.freeWorkers) == 0 {
			return nil
		}
		if wm.readyQueue.Len() == 0 {
			return nil
		}
		j := heap.Pop(&wm.readyQueue).(*job)
		logf("run: %s", j.n.Output)

		if wm.para != nil {
			var err error
			j.runners, err = j.createRunners()
			if err != nil {
				return err
			}
			if len(j.runners) == 0 {
				wm.updateParents(j)
				wm.finishCnt++
			} else {
				wm.runnings[j.n.Output] = j
				wm.para.RunCommand(j.runners)
			}
		} else {
			j.numDeps = -1 // Do not let other workers pick this.
			w := wm.freeWorkers[0]
			wm.freeWorkers = wm.freeWorkers[1:]
			wm.busyWorkers[w] = true
			w.jobChan <- j
		}
	}
}

func (wm *workerManager) updateParents(j *job) {
	for _, p := range j.parents {
		p.numDeps--
		logf("child: %s (%d)", p.n.Output, p.numDeps)
		if p.depsTs < j.outputTs {
			p.depsTs = j.outputTs
		}
		wm.maybePushToReadyQueue(p)
	}
}

type workerManager struct {
	maxJobs     int
	jobs        []*job
	readyQueue  jobQueue
	jobChan     chan *job
	resultChan  chan jobResult
	newDepChan  chan newDep
	stopChan    chan bool
	waitChan    chan bool
	doneChan    chan error
	freeWorkers []*worker
	busyWorkers map[*worker]bool
	ex          *Executor
	para        *paraWorker
	paraChan    chan *paraResult
	runnings    map[string]*job

	finishCnt int
}

func newWorkerManager(numJobs int, paraPath string) (*workerManager, error) {
	wm := &workerManager{
		maxJobs:     numJobs,
		jobChan:     make(chan *job),
		resultChan:  make(chan jobResult),
		newDepChan:  make(chan newDep),
		stopChan:    make(chan bool),
		waitChan:    make(chan bool),
		doneChan:    make(chan error),
		busyWorkers: make(map[*worker]bool),
	}

	if paraPath != "" {
		wm.runnings = make(map[string]*job)
		wm.paraChan = make(chan *paraResult)
		var err error
		wm.para, err = newParaWorker(wm.paraChan, numJobs, paraPath)
		if err != nil {
			return nil, err
		}
		go wm.para.Run()
	} else {
		wm.busyWorkers = make(map[*worker]bool)
		for i := 0; i < numJobs; i++ {
			w := newWorker(wm)
			wm.freeWorkers = append(wm.freeWorkers, w)
			go w.Run()
		}
	}
	heap.Init(&wm.readyQueue)
	go wm.Run()
	return wm, nil
}

func exitStatus(err error) int {
	if err == nil {
		return 0
	}
	exit := 1
	if err, ok := err.(*exec.ExitError); ok {
		if w, ok := err.ProcessState.Sys().(syscall.WaitStatus); ok {
			return w.ExitStatus()
		}
	}
	return exit
}

func (wm *workerManager) hasTodo() bool {
	return wm.finishCnt != len(wm.jobs)
}

func (wm *workerManager) maybePushToReadyQueue(j *job) {
	if j.numDeps != 0 {
		return
	}
	heap.Push(&wm.readyQueue, j)
	logf("ready: %s", j.n.Output)
}

func (wm *workerManager) handleNewDep(j *job, neededBy *job) {
	if j.numDeps < 0 {
		neededBy.numDeps--
		if neededBy.id > 0 {
			panic("FIXME: already in WM... can this happen?")
		}
	} else {
		j.parents = append(j.parents, neededBy)
	}
}

func (wm *workerManager) Run() {
	done := false
	var err error
Loop:
	for wm.hasTodo() || len(wm.busyWorkers) > 0 || len(wm.runnings) > 0 || !done {
		select {
		case j := <-wm.jobChan:
			logf("wait: %s (%d)", j.n.Output, j.numDeps)
			j.id = len(wm.jobs) + 1
			wm.jobs = append(wm.jobs, j)
			wm.maybePushToReadyQueue(j)
		case jr := <-wm.resultChan:
			logf("done: %s", jr.j.n.Output)
			delete(wm.busyWorkers, jr.w)
			wm.freeWorkers = append(wm.freeWorkers, jr.w)
			wm.updateParents(jr.j)
			wm.finishCnt++
			if jr.err != nil {
				err = jr.err
				close(wm.stopChan)
				break Loop
			}
		case af := <-wm.newDepChan:
			wm.handleNewDep(af.j, af.neededBy)
			logf("dep: %s (%d) %s", af.neededBy.n.Output, af.neededBy.numDeps, af.j.n.Output)
		case pr := <-wm.paraChan:
			if pr.status < 0 && pr.signal < 0 {
				j := wm.runnings[pr.output]
				for _, r := range j.runners {
					if r.echo || DryRunFlag {
						fmt.Printf("%s\n", r.cmd)
					}
				}
			} else {
				fmt.Fprint(os.Stdout, pr.stdout)
				fmt.Fprint(os.Stderr, pr.stderr)
				j := wm.runnings[pr.output]
				wm.updateParents(j)
				delete(wm.runnings, pr.output)
				wm.finishCnt++
			}
		case done = <-wm.waitChan:
		}
		err = wm.handleJobs()
		if err != nil {
			break Loop
		}

		if wm.para != nil {
			numBusy := len(wm.runnings)
			if numBusy > wm.maxJobs {
				numBusy = wm.maxJobs
			}
			logf("job=%d ready=%d free=%d busy=%d", len(wm.jobs)-wm.finishCnt, wm.readyQueue.Len(), wm.maxJobs-numBusy, numBusy)
		} else {
			logf("job=%d ready=%d free=%d busy=%d", len(wm.jobs)-wm.finishCnt, wm.readyQueue.Len(), len(wm.freeWorkers), len(wm.busyWorkers))
		}
	}
	if !done {
		<-wm.waitChan
	}

	if wm.para != nil {
		logf("Wait for para to finish")
		err := wm.para.Wait()
		if err != nil {
			logf("para failed: %v", err)
		}
	} else {
		for _, w := range wm.freeWorkers {
			w.Wait()
		}
		for w := range wm.busyWorkers {
			w.Wait()
		}
	}
	wm.doneChan <- err
}

func (wm *workerManager) PostJob(j *job) error {
	select {
	case wm.jobChan <- j:
		return nil
	case <-wm.stopChan:
		return errors.New("worker manager stopped")
	}
}

func (wm *workerManager) ReportResult(w *worker, j *job, err error) {
	select {
	case wm.resultChan <- jobResult{w: w, j: j, err: err}:
	case <-wm.stopChan:
	}
}

func (wm *workerManager) ReportNewDep(j *job, neededBy *job) {
	select {
	case wm.newDepChan <- newDep{j: j, neededBy: neededBy}:
	case <-wm.stopChan:
	}
}

func (wm *workerManager) Wait() error {
	wm.waitChan <- true
	return <-wm.doneChan
}
