test: foo

foo: A:=FAIL
foo: A:=PASS
foo:
	echo $(A)
