package main

import "os"

func main() {
	mk, err := ParseDefaultMakefile()
	if err != nil {
		panic(err)
	}

	for _, stmt := range mk.stmts {
		stmt.show()
	}

	vars := make(map[string]string)
	er, err := Eval(mk, vars)
	if err != nil {
		panic(err)
	}
	Exec(er, os.Args[1:])
}
