varname_with_ws:=hello, world!
$(varname_with_ws):=PASS
foo bar = PASS2
all:
	echo $(hello, world!)
	echo $(foo bar)
