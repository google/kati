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

package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"text/template"
	"time"

	"github.com/google/kati"
)

const shellDateTimeformat = time.RFC3339

var (
	makefileFlag string
	jobsFlag     int

	loadJSON string
	saveJSON string
	loadGOB  string
	saveGOB  string
	useCache bool

	cpuprofile          string
	heapprofile         string
	memstats            string
	traceEventFile      string
	syntaxCheckOnlyFlag bool
	queryFlag           string
	eagerCmdEvalFlag    bool
	generateNinja       bool
	gomaDir             string
	usePara             bool
	findCachePrunes     string
	findCacheLeafNames  string
	shellDate           string
)

func init() {
	// TODO: Make this default and replace this by -d flag.
	flag.StringVar(&makefileFlag, "f", "", "Use it as a makefile")
	flag.IntVar(&jobsFlag, "j", 1, "Allow N jobs at once.")

	flag.StringVar(&loadGOB, "load", "", "")
	flag.StringVar(&saveGOB, "save", "", "")
	flag.StringVar(&loadJSON, "load_json", "", "")
	flag.StringVar(&saveJSON, "save_json", "", "")
	flag.BoolVar(&useCache, "use_cache", false, "Use cache.")

	flag.StringVar(&cpuprofile, "kati_cpuprofile", "", "write cpu profile to `file`")
	flag.StringVar(&heapprofile, "kati_heapprofile", "", "write heap profile to `file`")
	flag.StringVar(&memstats, "kati_memstats", "", "Show memstats with given templates")
	flag.StringVar(&traceEventFile, "kati_trace_event", "", "write trace event to `file`")
	flag.BoolVar(&syntaxCheckOnlyFlag, "c", false, "Syntax check only.")
	flag.StringVar(&queryFlag, "query", "", "Show the target info")
	flag.BoolVar(&eagerCmdEvalFlag, "eager_cmd_eval", false, "Eval commands first.")
	flag.BoolVar(&generateNinja, "ninja", false, "Generate build.ninja.")
	flag.StringVar(&gomaDir, "goma_dir", "", "If specified, use goma to build C/C++ files.")
	flag.BoolVar(&usePara, "use_para", false, "Use para.")

	flag.StringVar(&findCachePrunes, "find_cache_prunes", "",
		"space separated prune directories for find cache.")
	flag.StringVar(&findCacheLeafNames, "find_cache_leaf_names", "",
		"space separated leaf names for find cache.")
	flag.StringVar(&shellDate, "shell_date", "", "specify $(shell date) time as "+shellDateTimeformat)

	flag.BoolVar(&kati.LogFlag, "kati_log", false, "Verbose kati specific log")
	flag.BoolVar(&kati.StatsFlag, "kati_stats", false, "Show a bunch of statistics")
	flag.BoolVar(&kati.PeriodicStatsFlag, "kati_periodic_stats", false, "Show a bunch of periodic statistics")
	flag.BoolVar(&kati.EvalStatsFlag, "kati_eval_stats", false, "Show eval statistics")

	flag.BoolVar(&kati.DryRunFlag, "n", false, "Only print the commands that would be executed")

	// TODO: Make this default.
	flag.BoolVar(&kati.UseFindCache, "use_find_cache", false, "Use find cache.")
	flag.BoolVar(&kati.UseWildcardCache, "use_wildcard_cache", true, "Use wildcard cache.")
	flag.BoolVar(&kati.UseShellBuiltins, "use_shell_builtins", true, "Use shell builtins")
	flag.StringVar(&kati.IgnoreOptionalInclude, "ignore_optional_include", "", "If specified, skip reading -include directives start with the specified path.")
}

func writeHeapProfile() {
	f, err := os.Create(heapprofile)
	if err != nil {
		panic(err)
	}
	pprof.WriteHeapProfile(f)
	f.Close()
}

type memStatsDumper struct {
	*template.Template
}

func (t memStatsDumper) dump() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	var buf bytes.Buffer
	err := t.Template.Execute(&buf, ms)
	fmt.Println(buf.String())
	if err != nil {
		panic(err)
	}
}

func findPara() string {
	switch runtime.GOOS {
	case "linux":
		katicmd, err := os.Readlink("/proc/self/exe")
		if err != nil {
			panic(err)
		}
		return filepath.Join(filepath.Dir(katicmd), "para")
	default:
		panic(fmt.Sprintf("unknown OS: %s", runtime.GOOS))
	}
}

func load(req kati.LoadReq) (*kati.DepGraph, error) {
	startTime := time.Now()

	if loadGOB != "" {
		g, err := kati.GOB.Load(loadGOB)
		kati.LogStats("deserialize time: %q", time.Since(startTime))
		return g, err
	}
	if loadJSON != "" {
		g, err := kati.JSON.Load(loadJSON)
		kati.LogStats("deserialize time: %q", time.Since(startTime))
		return g, err
	}
	return kati.Load(req)
}

func save(g *kati.DepGraph, targets []string) error {
	var err error
	startTime := time.Now()
	if saveGOB != "" {
		err = kati.GOB.Save(g, saveGOB, targets)
		kati.LogStats("serialize time: %q", time.Since(startTime))
	}
	if saveJSON != "" {
		serr := kati.JSON.Save(g, saveJSON, targets)
		kati.LogStats("serialize time: %q", time.Since(startTime))
		if err == nil {
			err = serr
		}
	}

	if useCache && !g.IsCached() {
		kati.DumpDepGraphCache(g, targets)
		kati.LogStats("serialize time: %q", time.Since(startTime))
	}
	return err
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	flag.Parse()
	if cpuprofile != "" {
		f, err := os.Create(cpuprofile)
		if err != nil {
			panic(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
		kati.AtError(pprof.StopCPUProfile)
	}
	if heapprofile != "" {
		defer writeHeapProfile()
		kati.AtError(writeHeapProfile)
	}
	defer kati.DumpStats()
	kati.AtError(kati.DumpStats)
	if memstats != "" {
		ms := memStatsDumper{
			Template: template.Must(template.New("memstats").Parse(memstats)),
		}
		ms.dump()
		defer ms.dump()
		kati.AtError(ms.dump)
	}
	if traceEventFile != "" {
		f, err := os.Create(traceEventFile)
		if err != nil {
			panic(err)
		}
		kati.TraceEventStart(f)
		defer kati.TraceEventStop()
		kati.AtError(kati.TraceEventStop)
	}

	if shellDate != "" {
		if shellDate == "ref" {
			shellDate = shellDateTimeformat[:20] // until Z, drop 07:00
		}
		t, err := time.Parse(shellDateTimeformat, shellDate)
		if err != nil {
			panic(err)
		}
		kati.ShellDateTimestamp = t
	}

	var leafNames []string
	if findCacheLeafNames != "" {
		leafNames = strings.Fields(findCacheLeafNames)
	}
	if findCachePrunes != "" {
		kati.UseFindCache = true
		kati.AndroidFindCacheInit(strings.Fields(findCachePrunes), leafNames)
	}

	req := kati.FromCommandLine(flag.Args())
	if makefileFlag != "" {
		req.Makefile = makefileFlag
	}
	req.EnvironmentVars = os.Environ()
	req.UseCache = useCache

	g, err := load(req)
	if err != nil {
		panic(err)
	}
	nodes := g.Nodes()
	vars := g.Vars()

	if eagerCmdEvalFlag {
		startTime := time.Now()
		kati.EvalCommands(nodes, vars)
		kati.LogStats("eager eval command time: %q", time.Since(startTime))
	}

	err = save(g, req.Targets)
	if err != nil {
		panic(err)
	}

	if generateNinja {
		startTime := time.Now()
		kati.GenerateNinja(g, gomaDir)
		kati.LogStats("generate ninja time: %q", time.Since(startTime))
		return
	}

	if syntaxCheckOnlyFlag {
		return
	}

	if queryFlag != "" {
		kati.Query(os.Stdout, queryFlag, g)
		return
	}

	// TODO: Handle target specific variables.
	ev := kati.NewEvaluator(vars)
	for name, export := range g.Exports() {
		if export {
			os.Setenv(name, ev.EvaluateVar(name))
		} else {
			os.Unsetenv(name)
		}
	}

	startTime := time.Now()
	execOpt := &kati.ExecutorOpt{
		NumJobs: jobsFlag,
	}
	if usePara {
		execOpt.ParaPath = findPara()
	}
	ex := kati.NewExecutor(vars, execOpt)
	err = ex.Exec(nodes)
	if err != nil {
		panic(err)
	}
	kati.LogStats("exec time: %q", time.Since(startTime))

}
