# TODO(go): Fix

test: foo.x bar.z

Z:=FAIL
foo.x: X:=PASS

%.x: X+=FAIL
%.x: Y:=PASS
%.x: Z:=PASS

%.x:
	echo X=$(X) Y=$(Y) Z=$(Z)

X:=FAIL
%.z: X:=PASS
%.z:
	echo $(X)
