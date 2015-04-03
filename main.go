package main

import "flag"

var noKatiLogFlag bool
var makefileFlag string
var dryRunFlag bool

func parseFlags() {
	// TODO: Make this default and replace this by -d flag.
	flag.BoolVar(&noKatiLogFlag, "no_kati_log", false, "No verbose kati specific log")
	flag.StringVar(&makefileFlag, "f", "", "Use it as a makefile")

	flag.BoolVar(&dryRunFlag, "n", false, "Only print the commands that would be executed")
	flag.Parse()
}

func getBootstrapMakefile() Makefile {
	bootstrap := `
CC:=cc
CXX:=g++
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
	mk, err := ParseMakefileString(bootstrap, "*bootstrap*")
	if err != nil {
		panic(err)
	}
	return mk
}

func main() {
	parseFlags()

	bmk := getBootstrapMakefile()

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
	// TODO(ukai): environment variables.
	er, err := Eval(mk, vars)
	if err != nil {
		panic(err)
	}
	err = Exec(er, flag.Args())
	if err != nil {
		panic(err)
	}
}
