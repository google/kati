package main

func main() {
	mk, err := ParseDefaultMakefile()
	if err != nil {
		panic(err)
	}

	for _, stmt := range mk.stmts {
		stmt.show()
	}

	er := Eval(mk)
	Exec(er)
}
