test: foo.x

foo.x: X:=PASS

%.x: X+=FAIL
%.x: Y:=PASS

%.x:
	echo X=$(X) Y=$(Y)
