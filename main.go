package main

import (
	"flag"
)

var noKatiLogFlag bool

func parseFlags() {
	// TODO: Make this default and replace this by -d flag.
	flag.BoolVar(&noKatiLogFlag, "no_kati_log", false, "No verbose kati specific log")

	flag.Parse()
}

func main() {
	parseFlags()

	bmk := GetBootstrapMakefile()

	mk, err := ParseDefaultMakefile()
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
