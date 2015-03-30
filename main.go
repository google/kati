package main

func main() {
	mk, err := ParseDefaultMakefile()
	if err != nil {
		panic(err)
	}

	for _, stmt := range mk.stmts {
		stmt.show()
	}

	er, err := Eval(mk)
	if err != nil {
		panic(err)
	}
	Exec(er)
}
