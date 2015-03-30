test1: foo
	echo test1

test2: foo
	echo test2

foo:
	echo foo > $@
