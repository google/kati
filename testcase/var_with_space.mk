varname_with_ws:=hello, world!
$(varname_with_ws):=PASS
all:
	echo $(hello, world!)
