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
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"text/template"
	"time"

	"github.com/golang/glog"
	"github.com/google/kati/golang/kati"
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

	m2n  bool
	goma bool

	cpuprofile          string
	heapprofile         string
	memstats            string
	traceEventFile      string
	syntaxCheckOnlyFlag bool
	queryFlag           string
	eagerCmdEvalFlag    bool
	generateNinja       bool
	regenNinja          bool
	ninjaSuffix         string
	gomaDir             string
	detectAndroidEcho   bool
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

	flag.BoolVar(&m2n, "m2n", false, "m2n mode")
	flag.BoolVar(&goma, "goma", false, "ensure goma start")

	flag.StringVar(&cpuprofile, "kati_cpuprofile", "", "write cpu profile to `file`")
	flag.StringVar(&heapprofile, "kati_heapprofile", "", "write heap profile to `file`")
	flag.StringVar(&memstats, "kati_memstats", "", "Show memstats with given templates")
	flag.StringVar(&traceEventFile, "kati_trace_event", "", "write trace event to `file`")
	flag.BoolVar(&syntaxCheckOnlyFlag, "c", false, "Syntax check only.")
	flag.StringVar(&queryFlag, "query", "", "Show the target info")
	flag.BoolVar(&eagerCmdEvalFlag, "eager_cmd_eval", false, "Eval commands first.")
	flag.BoolVar(&generateNinja, "ninja", false, "Generate build.ninja.")
	flag.BoolVar(&regenNinja, "gen_regen_rule", false, "Generate regenerate build.ninja rule.")
	flag.StringVar(&ninjaSuffix, "ninja_suffix", "", "suffix for ninja files.")
	flag.StringVar(&gomaDir, "goma_dir", "", "If specified, use goma to build C/C++ files.")
	// TODO(ukai): implement --regen
	flag.BoolVar(&detectAndroidEcho, "detect_android_echo", false, "detect echo as ninja description.")

	flag.StringVar(&shellDate, "shell_date", "", "specify $(shell date) time as "+shellDateTimeformat)

	flag.BoolVar(&kati.StatsFlag, "kati_stats", false, "Show a bunch of statistics")
	flag.BoolVar(&kati.PeriodicStatsFlag, "kati_periodic_stats", false, "Show a bunch of periodic statistics")
	flag.BoolVar(&kati.EvalStatsFlag, "kati_eval_stats", false, "Show eval statistics")

	flag.BoolVar(&kati.DryRunFlag, "n", false, "Only print the commands that would be executed")

	// TODO: Make this default.
	flag.BoolVar(&kati.UseFindEmulator, "use_find_emulator", false, "use find emulator")
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

func load(req kati.LoadReq) (*kati.DepGraph, error) {
	if loadGOB != "" {
		g, err := kati.GOB.Load(loadGOB)
		return g, err
	}
	if loadJSON != "" {
		g, err := kati.JSON.Load(loadJSON)
		return g, err
	}
	g, err := kati.Load(req)
	return g, err
}

func save(g *kati.DepGraph, targets []string) error {
	var err error
	if saveGOB != "" {
		err = kati.GOB.Save(g, saveGOB, targets)
	}
	if saveJSON != "" {
		serr := kati.JSON.Save(g, saveJSON, targets)
		if err == nil {
			err = serr
		}
	}
	return err
}

func m2nsetup() {
	fmt.Println("kati: m2n mode")
	generateNinja = true
	kati.IgnoreOptionalInclude = "out/%.P"
	kati.UseFindEmulator = true
}

func gomasetup() {
	for _, k := range []string{"CC_WRAPPER", "CXX_WRAPPER", "JAVAC_WRAPPER"} {
		v := os.Getenv(k)
		if v != "" {
			fmt.Printf("Note: %s=%s may confuse m2n --goma, unsetting", k, v)
			os.Unsetenv(k)
		}
	}

	if gomaDir == "" {
		gomaDir = os.Getenv("GOMA_DIR")
		if gomaDir == "" {
			gomaDir = os.ExpandEnv("${HOME}/goma")
		}
	}
	fmt.Printf("kati: setup goma: %s\n", gomaDir)
	cmd := exec.Command(filepath.Join(gomaDir, "goma_ctl.py"), "ensure_start")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		fmt.Printf("goma failed to start: %v", err)
		os.Exit(1)
	}
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	m2ncmd := false
	if filepath.Base(os.Args[0]) == "m2n" {
		m2nsetup()
		m2ncmd = true
	}
	flag.Parse()
	args := flag.Args()
	if m2n {
		generateNinja = true
		if !m2ncmd {
			m2nsetup()
		}
		if len(args) > 1 {
			fmt.Println("use only first argument as ONE_SHOT_MAKEFILE. ignore rest")
		}
		if len(args) > 0 {
			err := os.Setenv("ONE_SHOT_MAKEFILE", filepath.Join(args[0], "Android.mk"))
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			fmt.Printf("ONE_SHOT_MAKEFILE=%s\n", os.ExpandEnv("${ONE_SHOT_MAKEFILE}"))
		}
		args = args[:0]
	}
	if goma {
		gomasetup()
	}
	err := katiMain(args)
	if err != nil {
		fmt.Println(err)
		// http://www.gnu.org/software/make/manual/html_node/Running.html
		os.Exit(2)
	}
}

func katiMain(args []string) error {
	defer glog.Flush()
	if cpuprofile != "" {
		f, err := os.Create(cpuprofile)
		if err != nil {
			return err
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	if heapprofile != "" {
		defer writeHeapProfile()
	}
	defer kati.DumpStats()
	if memstats != "" {
		ms := memStatsDumper{
			Template: template.Must(template.New("memstats").Parse(memstats)),
		}
		ms.dump()
		defer ms.dump()
	}
	if traceEventFile != "" {
		f, err := os.Create(traceEventFile)
		if err != nil {
			panic(err)
		}
		kati.TraceEventStart(f)
		defer kati.TraceEventStop()
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

	req := kati.FromCommandLine(args)
	if makefileFlag != "" {
		req.Makefile = makefileFlag
	}
	req.EnvironmentVars = os.Environ()
	req.UseCache = useCache
	req.EagerEvalCommand = eagerCmdEvalFlag

	g, err := load(req)
	if err != nil {
		return err
	}

	err = save(g, req.Targets)
	if err != nil {
		return err
	}

	if generateNinja {
		var args []string
		if regenNinja {
			args = os.Args
		}
		n := kati.NinjaGenerator{
			Args:              args,
			Suffix:            ninjaSuffix,
			GomaDir:           gomaDir,
			DetectAndroidEcho: detectAndroidEcho,
		}
		return n.Save(g, "", req.Targets)
	}

	if syntaxCheckOnlyFlag {
		return nil
	}

	if queryFlag != "" {
		kati.Query(os.Stdout, queryFlag, g)
		return nil
	}

	execOpt := &kati.ExecutorOpt{
		NumJobs: jobsFlag,
	}
	ex, err := kati.NewExecutor(execOpt)
	if err != nil {
		return err
	}
	err = ex.Exec(g, req.Targets)
	if err != nil {
		return err
	}
	return nil
}
