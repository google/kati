FOO:=BAR
test: $$(FOO) := FAIL
test:
	echo $(BAR)
