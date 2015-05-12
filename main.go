package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"text/template"
	"time"
)

var (
	katiLogFlag         bool
	makefileFlag        string
	dryRunFlag          bool
	jobsFlag            int
	cpuprofile          string
	heapprofile         string
	memstats            string
	katiStatsFlag       bool
	katiEvalStatsFlag   bool
	loadJson            string
	saveJson            string
	loadGob             string
	saveGob             string
	syntaxCheckOnlyFlag bool
	queryFlag           string
	eagerCmdEvalFlag    bool
)

func parseFlags() {
	// TODO: Make this default and replace this by -d flag.
	flag.BoolVar(&katiLogFlag, "kati_log", false, "Verbose kati specific log")
	flag.StringVar(&makefileFlag, "f", "", "Use it as a makefile")

	flag.BoolVar(&dryRunFlag, "n", false, "Only print the commands that would be executed")

	flag.IntVar(&jobsFlag, "j", 1, "Allow N jobs at once.")

	flag.StringVar(&loadGob, "load", "", "")
	flag.StringVar(&saveGob, "save", "", "")
	flag.StringVar(&loadJson, "load_json", "", "")
	flag.StringVar(&saveJson, "save_json", "", "")

	flag.StringVar(&cpuprofile, "kati_cpuprofile", "", "write cpu profile to `file`")
	flag.StringVar(&heapprofile, "kati_heapprofile", "", "write heap profile to `file`")
	flag.StringVar(&memstats, "kati_memstats", "", "Show memstats with given templates")
	flag.BoolVar(&katiStatsFlag, "kati_stats", false, "Show a bunch of statistics")
	flag.BoolVar(&katiEvalStatsFlag, "kati_eval_stats", false, "Show eval statistics")
	flag.BoolVar(&eagerCmdEvalFlag, "eager_cmd_eval", false, "Eval commands first.")
	flag.BoolVar(&syntaxCheckOnlyFlag, "c", false, "Syntax check only.")
	flag.StringVar(&queryFlag, "query", "", "Show the target info")
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
	bootstrap = fmt.Sprintf("%s\nMAKECMDGOALS:=%s\n", bootstrap, strings.Join(targets, " "))
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

func getDepGraph(clvars []string, targets []string) ([]*DepNode, Vars) {
	startTime := time.Now()

	if loadGob != "" {
		n, v := LoadDepGraph(loadGob)
		LogStats("deserialize time: %q", time.Now().Sub(startTime))
		return n, v
	}
	if loadJson != "" {
		n, v := LoadDepGraphFromJson(loadJson)
		LogStats("deserialize time: %q", time.Now().Sub(startTime))
		return n, v
	}

	bmk := getBootstrapMakefile(targets)

	var mk Makefile
	var err error
	if len(makefileFlag) > 0 {
		mk, err = ParseMakefile(makefileFlag)
	} else {
		mk, err = ParseDefaultMakefile()
	}
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
		Log("envvar %q", kv)
		if len(kv) < 2 {
			panic(fmt.Sprintf("A weird environ variable %q", kv))
		}
		vars.Assign(kv[0], RecursiveVar{
			expr:   literal(kv[1]),
			origin: "environment",
		})
	}
	vars.Assign("MAKEFILE_LIST", SimpleVar{value: []byte{}, origin: "file"})
	for _, v := range clvars {
		kv := strings.SplitN(v, "=", 2)
		Log("cmdlinevar %q", kv)
		if len(kv) < 2 {
			panic(fmt.Sprintf("unexpected command line var %q", kv))
		}
		vars.Assign(kv[0], RecursiveVar{
			expr:   literal(kv[1]),
			origin: "command line",
		})
	}

	er, err := Eval(mk, vars)
	if err != nil {
		panic(err)
	}

	vars.Merge(er.vars)

	LogStats("eval time: %q", time.Now().Sub(startTime))
	LogStats("shell func time: %q", shellFuncTime)

	startTime = time.Now()
	db := NewDepBuilder(er, vars)
	LogStats("dep build prepare time: %q", time.Now().Sub(startTime))

	startTime = time.Now()
	nodes, err2 := db.Eval(targets)
	if err2 != nil {
		panic(err2)
	}
	LogStats("dep build time: %q", time.Now().Sub(startTime))
	return nodes, vars
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
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

	clvars, targets := parseCommandLine()

	nodes, vars := getDepGraph(clvars, targets)

	if eagerCmdEvalFlag {
		startTime := time.Now()
		EvalCommands(nodes, vars)
		LogStats("eager eval command time: %q", time.Now().Sub(startTime))
	}

	if saveGob != "" {
		startTime := time.Now()
		DumpDepGraph(nodes, vars, saveGob)
		LogStats("serialize time: %q", time.Now().Sub(startTime))
	}
	if saveJson != "" {
		startTime := time.Now()
		DumpDepGraphAsJson(nodes, vars, saveJson)
		LogStats("serialize time: %q", time.Now().Sub(startTime))
	}

	if syntaxCheckOnlyFlag {
		return
	}

	if queryFlag != "" {
		HandleQuery(queryFlag, nodes, vars)
		return
	}

	startTime := time.Now()
	ex := NewExecutor(vars)
	err := ex.Exec(nodes)
	if err != nil {
		panic(err)
	}
	LogStats("exec time: %q", time.Now().Sub(startTime))
}
