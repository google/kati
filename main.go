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
	"crypto/sha1"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"text/template"
	"time"
)

const shellDateTimeformat = time.RFC3339

var (
	katiLogFlag           bool
	makefileFlag          string
	dryRunFlag            bool
	jobsFlag              int
	cpuprofile            string
	heapprofile           string
	memstats              string
	traceEventFile        string
	katiStatsFlag         bool
	katiPeriodicStatsFlag bool
	katiEvalStatsFlag     bool
	loadJSON              string
	saveJSON              string
	loadGOB               string
	saveGOB               string
	syntaxCheckOnlyFlag   bool
	queryFlag             string
	eagerCmdEvalFlag      bool
	useParaFlag           bool
	useCache              bool
	useFindCache          bool
	findCachePrunes       string
	findCacheLeafNames    string
	useWildcardCache      bool
	useShellBuiltins      bool
	shellDate             string
	generateNinja         bool
	ignoreOptionalInclude string
	gomaDir               string

	katiDir string
)

type DepGraph struct {
	nodes    []*DepNode
	vars     Vars
	readMks  []*ReadMakefile
	exports  map[string]bool
	isCached bool
}

func parseFlags() {
	// TODO: Make this default and replace this by -d flag.
	flag.BoolVar(&katiLogFlag, "kati_log", false, "Verbose kati specific log")
	flag.StringVar(&makefileFlag, "f", "", "Use it as a makefile")

	flag.BoolVar(&dryRunFlag, "n", false, "Only print the commands that would be executed")

	flag.IntVar(&jobsFlag, "j", 1, "Allow N jobs at once.")

	flag.StringVar(&loadGOB, "load", "", "")
	flag.StringVar(&saveGOB, "save", "", "")
	flag.StringVar(&loadJSON, "load_json", "", "")
	flag.StringVar(&saveJSON, "save_json", "", "")

	flag.StringVar(&cpuprofile, "kati_cpuprofile", "", "write cpu profile to `file`")
	flag.StringVar(&heapprofile, "kati_heapprofile", "", "write heap profile to `file`")
	flag.StringVar(&memstats, "kati_memstats", "", "Show memstats with given templates")
	flag.StringVar(&traceEventFile, "kati_trace_event", "", "write trace event to `file`")
	flag.BoolVar(&katiStatsFlag, "kati_stats", false, "Show a bunch of statistics")
	flag.BoolVar(&katiPeriodicStatsFlag, "kati_periodic_stats", false, "Show a bunch of periodic statistics")
	flag.BoolVar(&katiEvalStatsFlag, "kati_eval_stats", false, "Show eval statistics")
	flag.BoolVar(&eagerCmdEvalFlag, "eager_cmd_eval", false, "Eval commands first.")
	flag.BoolVar(&useParaFlag, "use_para", false, "Use para.")
	flag.BoolVar(&syntaxCheckOnlyFlag, "c", false, "Syntax check only.")
	flag.StringVar(&queryFlag, "query", "", "Show the target info")
	// TODO: Make this default.
	flag.BoolVar(&useCache, "use_cache", false, "Use cache.")
	flag.BoolVar(&useFindCache, "use_find_cache", false, "Use find cache.")
	flag.StringVar(&findCachePrunes, "find_cache_prunes", "",
		"space separated prune directories for find cache.")
	flag.StringVar(&findCacheLeafNames, "find_cache_leaf_names", "",
		"space separated leaf names for find cache.")
	flag.BoolVar(&useWildcardCache, "use_wildcard_cache", true, "Use wildcard cache.")
	flag.BoolVar(&useShellBuiltins, "use_shell_builtins", true, "Use shell builtins")
	flag.StringVar(&shellDate, "shell_date", "", "specify $(shell date) time as "+shellDateTimeformat)
	flag.BoolVar(&generateNinja, "ninja", false, "Generate build.ninja.")
	flag.StringVar(&ignoreOptionalInclude, "ignore_optional_include", "", "If specified, skip reading -include directives start with the specified path.")
	flag.StringVar(&gomaDir, "goma_dir", "", "If specified, use goma to build C/C++ files.")
	flag.Parse()
}

func parseCommandLine() ([]string, []string) {
	var vars []string
	var targets []string
	for _, arg := range flag.Args() {
		if strings.IndexByte(arg, '=') >= 0 {
			vars = append(vars, arg)
			continue
		}
		targets = append(targets, arg)
	}
	return vars, targets
}

func getBootstrapMakefile(targets []string) Makefile {
	bootstrap := `
CC:=cc
CXX:=g++
AR:=ar
MAKE:=kati
# Pretend to be GNU make 3.81, for compatibility.
MAKE_VERSION:=3.81
SHELL:=/bin/sh
# TODO: Add more builtin vars.

# http://www.gnu.org/software/make/manual/make.html#Catalogue-of-Rules
# The document above is actually not correct. See default.c:
# http://git.savannah.gnu.org/cgit/make.git/tree/default.c?id=4.1
.c.o:
	$(CC) $(CFLAGS) $(CPPFLAGS) $(TARGET_ARCH) -c -o $@ $<
.cc.o:
	$(CXX) $(CXXFLAGS) $(CPPFLAGS) $(TARGET_ARCH) -c -o $@ $<
# TODO: Add more builtin rules.
`
	bootstrap += fmt.Sprintf("MAKECMDGOALS:=%s\n", strings.Join(targets, " "))
	cwd, err := filepath.Abs(".")
	if err != nil {
		panic(err)
	}
	bootstrap += fmt.Sprintf("CURDIR:=%s\n", cwd)
	mk, err := ParseMakefileString(bootstrap, BootstrapMakefile, 0)
	if err != nil {
		panic(err)
	}
	return mk
}

func maybeWriteHeapProfile() {
	if heapprofile != "" {
		f, err := os.Create(heapprofile)
		if err != nil {
			panic(err)
		}
		pprof.WriteHeapProfile(f)
	}
}

func getDepGraph(clvars []string, targets []string) *DepGraph {
	startTime := time.Now()

	if loadGOB != "" {
		g := LoadDepGraph(loadGOB)
		LogStats("deserialize time: %q", time.Since(startTime))
		return g
	}
	if loadJSON != "" {
		g := LoadDepGraphFromJSON(loadJSON)
		LogStats("deserialize time: %q", time.Since(startTime))
		return g
	}

	makefile := makefileFlag
	if makefile == "" {
		makefile = GetDefaultMakefile()
	}

	if useCache {
		g := LoadDepGraphCache(makefile, targets)
		if g != nil {
			return g
		}
	}

	bmk := getBootstrapMakefile(targets)

	content, err := ioutil.ReadFile(makefile)
	if err != nil {
		panic(err)
	}
	mk, err := ParseMakefile(content, makefile)
	if err != nil {
		panic(err)
	}

	for _, stmt := range mk.stmts {
		stmt.show()
	}

	mk.stmts = append(bmk.stmts, mk.stmts...)

	vars := make(Vars)
	for _, env := range os.Environ() {
		kv := strings.SplitN(env, "=", 2)
		Logf("envvar %q", kv)
		if len(kv) < 2 {
			panic(fmt.Sprintf("A weird environ variable %q", kv))
		}
		vars.Assign(kv[0], &RecursiveVar{
			expr:   literal(kv[1]),
			origin: "environment",
		})
	}
	vars.Assign("MAKEFILE_LIST", &SimpleVar{value: "", origin: "file"})
	for _, v := range clvars {
		kv := strings.SplitN(v, "=", 2)
		Logf("cmdlinevar %q", kv)
		if len(kv) < 2 {
			panic(fmt.Sprintf("unexpected command line var %q", kv))
		}
		vars.Assign(kv[0], &RecursiveVar{
			expr:   literal(kv[1]),
			origin: "command line",
		})
	}

	er, err := Eval(mk, vars)
	if err != nil {
		panic(err)
	}

	vars.Merge(er.vars)

	LogStats("eval time: %q", time.Since(startTime))
	LogStats("shell func time: %q %d", shellStats.duration, shellStats.count)

	startTime = time.Now()
	db := NewDepBuilder(er, vars)
	LogStats("dep build prepare time: %q", time.Since(startTime))

	startTime = time.Now()
	nodes, err2 := db.Eval(targets)
	if err2 != nil {
		panic(err2)
	}
	LogStats("dep build time: %q", time.Since(startTime))
	var readMks []*ReadMakefile
	// Always put the root Makefile as the first element.
	readMks = append(readMks, &ReadMakefile{
		Filename: makefile,
		Hash:     sha1.Sum(content),
		State:    FileExists,
	})
	readMks = append(readMks, er.readMks...)
	return &DepGraph{
		nodes:   nodes,
		vars:    vars,
		readMks: readMks,
		exports: er.exports,
	}
}

func findKatiDir() {
	switch runtime.GOOS {
	case "linux":
		kati, err := os.Readlink("/proc/self/exe")
		if err != nil {
			panic(err)
		}
		katiDir = filepath.Dir(kati)
	default:
		panic(fmt.Sprintf("unknown OS: %s", runtime.GOOS))
	}
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	findKatiDir()
	parseFlags()
	if cpuprofile != "" {
		f, err := os.Create(cpuprofile)
		if err != nil {
			panic(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	defer maybeWriteHeapProfile()
	defer dumpStats()
	if memstats != "" {
		t := template.Must(template.New("memstats").Parse(memstats))
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		var buf bytes.Buffer
		err := t.Execute(&buf, ms)
		fmt.Println(buf.String())
		if err != nil {
			panic(err)
		}
		defer func() {
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			var buf bytes.Buffer
			t.Execute(&buf, ms)
			fmt.Println(buf.String())
		}()
	}
	if traceEventFile != "" {
		f, err := os.Create(traceEventFile)
		if err != nil {
			panic(err)
		}
		traceEvent.start(f)
		defer traceEvent.stop()
	}

	if shellDate != "" {
		if shellDate == "ref" {
			shellDate = shellDateTimeformat[:20] // until Z, drop 07:00
		}
		t, err := time.Parse(shellDateTimeformat, shellDate)
		if err != nil {
			panic(err)
		}
		shellDateTimestamp = t
	}

	if findCacheLeafNames != "" {
		androidDefaultLeafNames = strings.Fields(findCacheLeafNames)
	}
	if findCachePrunes != "" {
		useFindCache = true
		androidFindCache.init(strings.Fields(findCachePrunes), androidDefaultLeafNames)
	}

	clvars, targets := parseCommandLine()

	g := getDepGraph(clvars, targets)
	nodes := g.nodes
	vars := g.vars

	if eagerCmdEvalFlag {
		startTime := time.Now()
		EvalCommands(nodes, vars)
		LogStats("eager eval command time: %q", time.Since(startTime))
	}

	if saveGOB != "" {
		startTime := time.Now()
		DumpDepGraph(g, saveGOB, targets)
		LogStats("serialize time: %q", time.Since(startTime))
	}
	if saveJSON != "" {
		startTime := time.Now()
		DumpDepGraphAsJSON(g, saveJSON, targets)
		LogStats("serialize time: %q", time.Since(startTime))
	}

	if useCache && !g.isCached {
		startTime := time.Now()
		DumpDepGraphCache(g, targets)
		LogStats("serialize time: %q", time.Since(startTime))
	}

	if generateNinja {
		startTime := time.Now()
		GenerateNinja(g)
		LogStats("generate ninja time: %q", time.Since(startTime))
		return
	}

	if syntaxCheckOnlyFlag {
		return
	}

	if queryFlag != "" {
		HandleQuery(queryFlag, g)
		return
	}

	// TODO: Handle target specific variables.
	ev := newEvaluator(vars)
	for name, export := range g.exports {
		if export {
			os.Setenv(name, ev.EvaluateVar(name))
		} else {
			os.Unsetenv(name)
		}
	}

	startTime := time.Now()
	ex := NewExecutor(vars)
	err := ex.Exec(nodes)
	if err != nil {
		panic(err)
	}
	LogStats("exec time: %q", time.Since(startTime))
}
