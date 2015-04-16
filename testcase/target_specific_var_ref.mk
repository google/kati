test: foo bar

X:=FAIL
foo: X:=PASS
foo: A:=$(X)
foo:
	echo $(A)

Y:=PASS
bar: B:=$(Y)
bar:
	echo $(B)
