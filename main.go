package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

var katiLogFlag bool
var makefileFlag string
var dryRunFlag bool

func parseFlags() {
	// TODO: Make this default and replace this by -d flag.
	flag.BoolVar(&katiLogFlag, "kati_log", false, "Verbose kati specific log")
	flag.StringVar(&makefileFlag, "f", "", "Use it as a makefile")

	flag.BoolVar(&dryRunFlag, "n", false, "Only print the commands that would be executed")
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

func main() {
	parseFlags()

	clvars, targets := parseCommandLine()

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

	vars := NewVarTab(nil)
	for _, env := range os.Environ() {
		kv := strings.SplitN(env, "=", 2)
		Log("envvar %q", kv)
		if len(kv) < 2 {
			panic(fmt.Sprintf("A weird environ variable %q", kv))
		}
		vars.Assign(kv[0], RecursiveVar{
			expr:   kv[1],
			origin: "environment",
		})
	}
	vars.Assign("MAKEFILE_LIST", SimpleVar{value: "", origin: "file"})
	for _, v := range clvars {
		kv := strings.SplitN(v, "=", 2)
		Log("cmdlinevar %q", kv)
		if len(kv) < 2 {
			panic(fmt.Sprintf("unexpected command line var %q", kv))
		}
		vars.Assign(kv[0], RecursiveVar{
			expr:   kv[1],
			origin: "command line",
		})
	}

	// TODO(ukai): make variables in commandline.
	er, err := Eval(mk, vars)
	if err != nil {
		panic(err)
	}

	for k, v := range er.vars.Vars() {
		vars.Assign(k, v)
	}

	err = Exec(er, targets, vars)
	if err != nil {
		panic(err)
	}
}
