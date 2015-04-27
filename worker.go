package main

import (
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
}

type runner struct {
	output      string
	cmd         string
	echo        bool
	dryRun      bool
	ignoreError bool
	shell       string
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
			if !r.dryRun {
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
	ex := j.ex
	// For automatic variables.
	ex.currentOutput = j.n.Output
	ex.currentInputs = j.n.ActualInputs

	var restores []func()
	for k, v := range j.n.TargetSpecificVars {
		restores = append(restores, ex.vars.save(k))
		ex.vars[k] = v
	}
	defer func() {
		for _, restore := range restores {
			restore()
		}
	}()

	ev := newEvaluator(ex.vars)
	ev.filename = j.n.Filename
	ev.lineno = j.n.Lineno
	var runners []runner
	Log("Building: %s cmds:%q", j.n.Output, j.n.Cmds)
	r := runner{
		output: j.n.Output,
		echo:   true,
		dryRun: dryRunFlag,
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

func (j Job) build() error {
	if j.n.IsPhony {
		j.outputTs = -2 // trigger cmd even if all inputs don't exist.
	} else {
		j.outputTs = getTimestamp(j.n.Output)
	}

	if !j.n.HasRule {
		if j.outputTs >= 0 || j.n.IsPhony {
			//ex.done[output] = outputTs
			//ex.noRuleCnt++
			//return outputTs, nil
			return nil
		}
		if len(j.parents) == 0 {
			ErrorNoLocation("*** No rule to make target %q.", j.n.Output)
		} else {
			ErrorNoLocation("*** No rule to make target %q, needed by %q.", j.n.Output, j.parents[0].n.Output)
		}
		return fmt.Errorf("no rule to make target %q", j.n.Output)
	}

	if j.outputTs >= j.depsTs {
		// TODO: stats.
		return nil
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
	return nil
}

func (j Job) run() error {
	if err := j.build(); err != nil {
		return err
	}

	for _, p := range j.parents {
		p.numDeps--
		if p.depsTs < j.outputTs {
			p.depsTs = j.outputTs
		}
		/*
			if p.numDeps == 0 {
				p.run()
			}
		*/
	}
	return nil
}

type WorkerManager struct {
	jobs     []*Job
	jobChan  chan *Job
	waitChan chan bool
	doneChan chan bool
}

func NewWorkerManager() *WorkerManager {
	wm := &WorkerManager{
		jobChan:  make(chan *Job),
		waitChan: make(chan bool),
		doneChan: make(chan bool),
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

func (wm *WorkerManager) Run() {
	done := false
	for len(wm.jobs) > 0 || !done {
		select {
		case j := <-wm.jobChan:
			if j.numDeps == 0 {
				err := j.run()
				if err != nil {
					exit := exitStatus(err)
					ErrorNoLocation("[%s] Error %d: %v", j.n.Output, exit, err)
				}
			}
		case done = <-wm.waitChan:
		}
	}
	wm.doneChan <- true
}

func (wm *WorkerManager) PostJob(j *Job) {
	wm.jobChan <- j
}

func (wm *WorkerManager) Wait() {
	wm.waitChan <- true
	<-wm.doneChan
}
