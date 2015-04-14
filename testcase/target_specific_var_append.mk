# TODO: Fix

all: foo bar baz

foo: X:=PASS
foo: X+=

foo:
	echo $(X)

Y:=FAIL
bar: Y+=

bar:
	echo $(Y)
Y:=

Z:=FAIL
baz: Z?=PASS

baz:
	echo $(Z)
Z:=
