package main

import "os"

func main() {
	bmk := GetBootstrapMakefile()

	mk, err := ParseDefaultMakefile()
	if err != nil {
		panic(err)
	}

	for _, stmt := range mk.stmts {
		stmt.show()
	}

	mk.stmts = append(bmk.stmts, mk.stmts...)

	vars := make(map[string]string)
	// TODO(ukai): environment variables.
	er, err := Eval(mk, vars)
	if err != nil {
		panic(err)
	}
	err = Exec(er, os.Args[1:])
	if err != nil {
		panic(err)
	}
}
