A:= x:a:=foo;
B:=foo
BAR:=bar
BAZ:=baz
$(A) echo $(BAR) ; echo $(BAZ)
BAR:=FAIL_bar
BAZ:=FAIL_baz
x:
	echo $(a)
