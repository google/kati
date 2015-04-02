package main

import (
	"flag"
)

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

func main() {
	parseFlags()

	bmk := GetBootstrapMakefile()

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
