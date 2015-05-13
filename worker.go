package main

import (
	"container/heap"
	"fmt"
	"os"
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
	buf := newBuf()
	expr.Eval(buf, ev)
	cmds := buf.String()
	freeBuf(buf)
	var runners []runner
	for _, cmd := range strings.Split(cmds, "\n") {
		if len(runners) > 0 && strings.HasSuffix(runners[len(runners)-1].cmd, "\\") {
			runners[len(runners)-1].cmd += "\n"
			runners[len(runners)-1].cmd += cmd
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
	runners, _ := j.ex.createRunners(j.n, false)
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
		if !useParaFlag && len(wm.freeWorkers) == 0 {
			return
		}
		if wm.readyQueue.Len() == 0 {
			return
		}
		j := heap.Pop(&wm.readyQueue).(*Job)

		if useParaFlag {
			wm.runnings[j.n.Output] = j
			wm.para.RunCommand(j.createRunners())
		} else {
			j.numDeps = -1 // Do not let other workers pick this.
			w := wm.freeWorkers[0]
			wm.freeWorkers = wm.freeWorkers[1:]
			wm.busyWorkers[w] = true
			w.jobChan <- j
		}
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
	ex          *Executor
	para        *ParaWorker
	paraChan    chan *ParaResult
	runnings    map[string]*Job

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

	if useParaFlag {
		wm.runnings = make(map[string]*Job)
		wm.paraChan = make(chan *ParaResult)
		wm.para = NewParaWorker(wm.paraChan)
		go wm.para.Run()
	} else {
		wm.busyWorkers = make(map[*Worker]bool)
		for i := 0; i < jobsFlag; i++ {
			w := NewWorker(wm)
			wm.freeWorkers = append(wm.freeWorkers, w)
			go w.Run()
		}
	}
	heap.Init(&wm.readyQueue)
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
	for wm.hasTodo() || len(wm.busyWorkers) > 0 || len(wm.runnings) > 0 || !done {
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
		case pr := <-wm.paraChan:
			os.Stdout.Write([]byte(pr.stdout))
			os.Stderr.Write([]byte(pr.stderr))
			j := wm.runnings[pr.output]
			wm.updateParents(j)
			delete(wm.runnings, pr.output)
			wm.finishCnt++
		case done = <-wm.waitChan:
		}
		wm.handleJobs()

		if useParaFlag {
			numBusy := len(wm.runnings)
			if numBusy > jobsFlag {
				numBusy = jobsFlag
			}
			Log("job=%d ready=%d free=%d busy=%d", len(wm.jobs)-wm.finishCnt, wm.readyQueue.Len(), jobsFlag-numBusy, numBusy)
		} else {
			Log("job=%d ready=%d free=%d busy=%d", len(wm.jobs)-wm.finishCnt, wm.readyQueue.Len(), len(wm.freeWorkers), len(wm.busyWorkers))
		}
	}

	if useParaFlag {
		Log("Wait for para to finish")
		wm.para.Wait()
	} else {
		for _, w := range wm.freeWorkers {
			w.Wait()
		}
		for w := range wm.busyWorkers {
			w.Wait()
		}
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
