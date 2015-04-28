package main

import (
	"container/heap"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

type Job struct {
	n        *DepNode
	ex       *Executor
	parents  []*Job
	outputTs int64
	numDeps  int
	depsTs   int64
	id       int
}

type runner struct {
	output      string
	cmd         string
	echo        bool
	ignoreError bool
	shell       string
}

type JobResult struct {
	j *Job
	w *Worker
}

type NewDep struct {
	j        *Job
	neededBy *Job
}

type Worker struct {
	wm       *WorkerManager
	jobChan  chan *Job
	waitChan chan bool
	doneChan chan bool
}

type JobQueue []*Job

func (jq JobQueue) Len() int { return len(jq) }

func (jq JobQueue) Less(i, j int) bool {
	// First come, first serve, for GNU make compatibility.
	return jq[i].id < jq[j].id
}

func (jq JobQueue) Swap(i, j int) {
	jq[i], jq[j] = jq[j], jq[i]
}

func (jq *JobQueue) Push(x interface{}) {
	item := x.(*Job)
	*jq = append(*jq, item)
}

func (jq *JobQueue) Pop() interface{} {
	old := *jq
	n := len(old)
	item := old[n-1]
	*jq = old[0 : n-1]
	return item
}

func NewWorker(wm *WorkerManager) *Worker {
	w := &Worker{
		wm:       wm,
		jobChan:  make(chan *Job),
		waitChan: make(chan bool),
		doneChan: make(chan bool),
	}
	return w
}

func (w *Worker) Run() {
	done := false
	for !done {
		select {
		case j := <-w.jobChan:
			j.build()
			w.wm.ReportResult(w, j)
		case done = <-w.waitChan:
		}
	}
	w.doneChan <- true
}

func (w *Worker) PostJob(j *Job) {
	w.jobChan <- j
}

func (w *Worker) Wait() {
	w.waitChan <- true
	<-w.doneChan
}

func evalCmd(ev *Evaluator, r runner, s string) []runner {
	r = newRunner(r, s)
	if strings.IndexByte(r.cmd, '$') < 0 {
		// fast path
		return []runner{r}
	}
	// TODO(ukai): parse once more earlier?
	expr, _, err := parseExpr([]byte(r.cmd), nil)
	if err != nil {
		panic(fmt.Errorf("parse cmd %q: %v", r.cmd, err))
	}
	cmds := string(ev.Value(expr))
	var runners []runner
	for _, cmd := range strings.Split(cmds, "\n") {
		if len(runners) > 0 && strings.HasSuffix(runners[0].cmd, "\\") {
			runners[0].cmd += "\n"
			runners[0].cmd += cmd
		} else {
			runners = append(runners, newRunner(r, cmd))
		}
	}
	return runners
}

func newRunner(r runner, s string) runner {
	for {
		s = trimLeftSpace(s)
		if s == "" {
			return runner{}
		}
		switch s[0] {
		case '@':
			if !dryRunFlag {
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

func (r runner) run(output string) error {
	if r.echo || dryRunFlag {
		fmt.Printf("%s\n", r.cmd)
	}
	if dryRunFlag {
		return nil
	}
	args := []string{r.shell, "-c", r.cmd}
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

func (j Job) createRunners() []runner {
	var restores []func()
	defer func() {
		for _, restore := range restores {
			restore()
		}
	}()

	ex := j.ex
	ex.varsLock.Lock()
	restores = append(restores, func() { ex.varsLock.Unlock() })
	// For automatic variables.
	ex.currentOutput = j.n.Output
	ex.currentInputs = j.n.ActualInputs
	for k, v := range j.n.TargetSpecificVars {
		restores = append(restores, ex.vars.save(k))
		ex.vars[k] = v
	}

	ev := newEvaluator(ex.vars)
	ev.filename = j.n.Filename
	ev.lineno = j.n.Lineno
	var runners []runner
	Log("Building: %s cmds:%q", j.n.Output, j.n.Cmds)
	r := runner{
		output: j.n.Output,
		echo:   true,
		shell:  ex.shell,
	}
	for _, cmd := range j.n.Cmds {
		for _, r := range evalCmd(ev, r, cmd) {
			if len(r.cmd) != 0 {
				runners = append(runners, r)
			}
		}
	}
	return runners
}

func (j Job) build() {
	if j.n.IsPhony {
		j.outputTs = -2 // trigger cmd even if all inputs don't exist.
	} else {
		j.outputTs = getTimestamp(j.n.Output)
	}

	if !j.n.HasRule {
		if j.outputTs >= 0 || j.n.IsPhony {
			return
		}
		if len(j.parents) == 0 {
			ErrorNoLocation("*** No rule to make target %q.", j.n.Output)
		} else {
			ErrorNoLocation("*** No rule to make target %q, needed by %q.", j.n.Output, j.parents[0].n.Output)
		}
		ErrorNoLocation("no rule to make target %q", j.n.Output)
	}

	if j.outputTs >= j.depsTs {
		// TODO: stats.
		return
	}

	for _, r := range j.createRunners() {
		err := r.run(j.n.Output)
		if err != nil {
			exit := exitStatus(err)
			ErrorNoLocation("[%s] Error %d: %v", j.n.Output, exit, err)
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
}

func (wm *WorkerManager) handleJobs() {
	for {
		if len(wm.freeWorkers) == 0 {
			return
		}
		if wm.readyQueue.Len() == 0 {
			return
		}
		j := heap.Pop(&wm.readyQueue).(*Job)
		j.numDeps = -1 // Do not let other workers pick this.
		w := wm.freeWorkers[0]
		wm.freeWorkers = wm.freeWorkers[1:]
		wm.busyWorkers[w] = true
		w.jobChan <- j
	}
}

func (wm *WorkerManager) updateParents(j *Job) {
	for _, p := range j.parents {
		p.numDeps--
		if p.depsTs < j.outputTs {
			p.depsTs = j.outputTs
		}
		wm.maybePushToReadyQueue(p)
	}
}

type WorkerManager struct {
	jobs        []*Job
	readyQueue  JobQueue
	jobChan     chan *Job
	resultChan  chan JobResult
	newDepChan  chan NewDep
	waitChan    chan bool
	doneChan    chan bool
	freeWorkers []*Worker
	busyWorkers map[*Worker]bool

	finishCnt int
}

func NewWorkerManager() *WorkerManager {
	wm := &WorkerManager{
		jobChan:     make(chan *Job),
		resultChan:  make(chan JobResult),
		newDepChan:  make(chan NewDep),
		waitChan:    make(chan bool),
		doneChan:    make(chan bool),
		busyWorkers: make(map[*Worker]bool),
	}
	heap.Init(&wm.readyQueue)
	for i := 0; i < jobsFlag; i++ {
		w := NewWorker(wm)
		wm.freeWorkers = append(wm.freeWorkers, w)
		go w.Run()
	}
	go wm.Run()
	return wm
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

func (wm *WorkerManager) hasTodo() bool {
	return wm.finishCnt != len(wm.jobs)
}

func (wm *WorkerManager) maybePushToReadyQueue(j *Job) {
	if j.numDeps != 0 {
		return
	}
	heap.Push(&wm.readyQueue, j)
}

func (wm *WorkerManager) handleNewDep(j *Job, neededBy *Job) {
	if j.numDeps < 0 {
		neededBy.numDeps--
		if neededBy.id > 0 {
			panic("already in WM... can this happen?")
			wm.maybePushToReadyQueue(neededBy)
		}
	} else {
		j.parents = append(j.parents, neededBy)
	}
}

func (wm *WorkerManager) Run() {
	done := false
	for wm.hasTodo() || len(wm.busyWorkers) > 0 || !done {
		select {
		case j := <-wm.jobChan:
			j.id = len(wm.jobs) + 1
			wm.jobs = append(wm.jobs, j)
			wm.maybePushToReadyQueue(j)
		case jr := <-wm.resultChan:
			delete(wm.busyWorkers, jr.w)
			wm.freeWorkers = append(wm.freeWorkers, jr.w)
			wm.updateParents(jr.j)
			wm.finishCnt++
		case af := <-wm.newDepChan:
			wm.handleNewDep(af.j, af.neededBy)
		case done = <-wm.waitChan:
		}
		wm.handleJobs()
		Log("job=%d ready=%d free=%d busy=%d", len(wm.jobs)-wm.finishCnt, wm.readyQueue.Len(), len(wm.freeWorkers), len(wm.busyWorkers))
	}

	for _, w := range wm.freeWorkers {
		w.Wait()
	}
	for w := range wm.busyWorkers {
		w.Wait()
	}
	wm.doneChan <- true
}

func (wm *WorkerManager) PostJob(j *Job) {
	wm.jobChan <- j
}

func (wm *WorkerManager) ReportResult(w *Worker, j *Job) {
	wm.resultChan <- JobResult{w: w, j: j}
}

func (wm *WorkerManager) ReportNewDep(j *Job, neededBy *Job) {
	wm.newDepChan <- NewDep{j: j, neededBy: neededBy}
}

func (wm *WorkerManager) Wait() {
	wm.waitChan <- true
	<-wm.doneChan
}
