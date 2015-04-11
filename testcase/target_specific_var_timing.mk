PASS:=PASS
FAIL:=FAIL
PASS2:=PASS

test: foo

foo: X := $(PASS)
foo: Y=$(FAIL)
foo: Z=$(PASS2)

foo:
	echo $(X) $(Y) $(Z)

PASS:=
FAIL:=
