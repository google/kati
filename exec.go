package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
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

type AutoVar struct{ ex *Executor }

func (v AutoVar) Flavor() string  { return "undefined" }
func (v AutoVar) Origin() string  { return "automatic" }
func (v AutoVar) IsDefined() bool { panic("not implemented") }
func (v AutoVar) String() string  { panic("not implemented") }
func (v AutoVar) Append(*Evaluator, string) Var {
	panic("must not be called")
}
func (v AutoVar) AppendVar(*Evaluator, Var) Var {
	panic("must not be called")
}
func (v AutoVar) Serialize() SerializableVar {
	panic(fmt.Sprintf("cannot serialize auto var: %q", v))
}

type AutoAtVar struct{ AutoVar }

func (v AutoAtVar) Eval(w io.Writer, ev *Evaluator) {
	fmt.Fprint(w, v.ex.currentOutput)
}

type AutoLessVar struct{ AutoVar }

func (v AutoLessVar) Eval(w io.Writer, ev *Evaluator) {
	if len(v.ex.currentInputs) > 0 {
		fmt.Fprint(w, v.ex.currentInputs[0])
	}
}

type AutoHatVar struct{ AutoVar }

func (v AutoHatVar) Eval(w io.Writer, ev *Evaluator) {
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

type AutoPlusVar struct{ AutoVar }

func (v AutoPlusVar) Eval(w io.Writer, ev *Evaluator) {
	fmt.Fprint(w, strings.Join(v.ex.currentInputs, " "))
}

type AutoStarVar struct{ AutoVar }

func (v AutoStarVar) Eval(w io.Writer, ev *Evaluator) {
	// TODO: Use currentStem. See auto_stem_var.mk
	fmt.Fprint(w, stripExt(v.ex.currentOutput))
}

type AutoSuffixDVar struct {
	AutoVar
	v Var
}

func (v AutoSuffixDVar) Eval(w io.Writer, ev *Evaluator) {
	var buf bytes.Buffer
	v.v.Eval(&buf, ev)
	for i, tok := range splitSpaces(buf.String()) {
		if i > 0 {
			w.Write([]byte{' '})
		}
		fmt.Fprint(w, filepath.Dir(tok))
	}
}

type AutoSuffixFVar struct {
	AutoVar
	v Var
}

func (v AutoSuffixFVar) Eval(w io.Writer, ev *Evaluator) {
	var buf bytes.Buffer
	v.v.Eval(&buf, ev)
	for i, tok := range splitSpaces(buf.String()) {
		if i > 0 {
			w.Write([]byte{' '})
		}
		fmt.Fprint(w, filepath.Base(tok))
	}
}

// TODO(ukai): use time.Time?
func getTimestamp(filename string) int64 {
	st, err := os.Stat(filename)
	if err != nil {
		return -2
	}
	return st.ModTime().Unix()
}

func (ex *Executor) makeJobs(n *DepNode, neededBy *Job) error {
	output := n.Output
	if neededBy != nil {
		Log("MakeJob: %s for %s", output, neededBy.n.Output)
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
			Log("%s already done: %d", j.n.Output, j.outputTs)
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
	if !katiLogFlag && !katiStatsFlag {
		return
	}

	LogStats("build=%d alreadyDone=%d noRule=%d, upToDate=%d runCommand=%d",
		ex.buildCnt, ex.alreadyDoneCnt, ex.noRuleCnt, ex.upToDateCnt, ex.runCommandCnt)
	if len(ex.trace) > 1 {
		LogStats("trace=%q", ex.trace)
	}
}

func NewExecutor(vars Vars) *Executor {
	ex := &Executor{
		rules:       make(map[string]*Rule),
		suffixRules: make(map[string][]*Rule),
		done:        make(map[string]*Job),
		vars:        vars,
		wm:          NewWorkerManager(),
	}
	// TODO: We should move this to somewhere around evalCmd so that
	// we can handle SHELL in target specific variables.
	shellVar := ex.vars.Lookup("SHELL")
	ex.shell = shellVar.String()
	for k, v := range map[string]Var{
		"@": AutoAtVar{AutoVar: AutoVar{ex: ex}},
		"<": AutoLessVar{AutoVar: AutoVar{ex: ex}},
		"^": AutoHatVar{AutoVar: AutoVar{ex: ex}},
		"+": AutoPlusVar{AutoVar: AutoVar{ex: ex}},
		"*": AutoStarVar{AutoVar: AutoVar{ex: ex}},
	} {
		ex.vars[k] = v
		ex.vars[k+"D"] = AutoSuffixDVar{v: v}
		ex.vars[k+"F"] = AutoSuffixFVar{v: v}
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
