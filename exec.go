package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type Executor struct {
	rules         map[string]*Rule
	implicitRules []*Rule
	suffixRules   map[string][]*Rule
	firstRule     *Rule
	shell         string
	vars          Vars
	// target -> timestamp, a negative timestamp means the target is
	// currently being processed.
	done map[string]int64

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

func (ex *Executor) build(n *DepNode, neededBy string) (int64, error) {
	output := n.Output
	Log("Building: %s", output)
	ex.buildCnt++
	if ex.buildCnt%100 == 0 {
		ex.reportStats()
	}

	outputTs, ok := ex.done[output]
	if ok {
		if outputTs < 0 {
			fmt.Printf("Circular %s <- %s dependency dropped.\n", neededBy, output)
		}
		Log("Building: %s already done: %d", output, outputTs)
		ex.alreadyDoneCnt++
		return outputTs, nil
	}
	ex.done[output] = -1
	outputTs = getTimestamp(output)

	if !n.HasRule {
		if outputTs >= 0 {
			ex.done[output] = outputTs
			ex.noRuleCnt++
			return outputTs, nil
		}
		if neededBy == "" {
			ErrorNoLocation("*** No rule to make target %q.", output)
		} else {
			ErrorNoLocation("*** No rule to make target %q, needed by %q.", output, neededBy)
		}
		return outputTs, fmt.Errorf("no rule to make target %q", output)
	}

	latest := int64(-1)
	Log("Building: %s inputs:%q", output, n.Deps)
	for _, d := range n.Deps {
		if d.IsOrderOnly && exists(d.Output) {
			continue
		}

		ex.trace = append(ex.trace, d.Output)
		ts, err := ex.build(d, output)
		ex.trace = ex.trace[0 : len(ex.trace)-1]
		if err != nil {
			return outputTs, err
		}
		if latest < ts {
			latest = ts
		}
	}

	if outputTs >= latest {
		ex.done[output] = outputTs
		ex.upToDateCnt++
		return outputTs, nil
	}

	// For automatic variables.
	ex.currentOutput = output
	ex.currentInputs = n.ActualInputs

	var restores []func()
	for k, v := range n.TargetSpecificVars {
		restores = append(restores, ex.vars.save(k))
		ex.vars[k] = v
	}
	defer func() {
		for _, restore := range restores {
			restore()
		}
	}()

	ev := newEvaluator(ex.vars)
	ev.filename = n.Filename
	ev.lineno = n.Lineno
	var runners []runner
	Log("Building: %s cmds:%q", output, n.Cmds)
	r := runner{
		output: output,
		echo:   true,
		dryRun: dryRunFlag,
		shell:  ex.shell,
	}
	for _, cmd := range n.Cmds {
		for _, r := range evalCmd(ev, r, cmd) {
			if len(r.cmd) != 0 {
				runners = append(runners, r)
			}
		}
	}

	for _, r := range runners {
		err := r.run(output)
		if err != nil {
			exit := exitStatus(err)
			fmt.Printf("[%s] Error %d: %v\n", output, exit, err)
			return outputTs, err
		}
	}

	outputTs = getTimestamp(output)
	if outputTs < 0 {
		outputTs = time.Now().Unix()
	}
	ex.done[output] = outputTs
	Log("Building: %s done %d", output, outputTs)
	ex.runCommandCnt++
	return outputTs, nil
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
		done:        make(map[string]int64),
		vars:        vars,
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
		ex.build(root, "")
	}
	return nil
}
