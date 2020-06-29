# TODO(c/test2): bar is built even if foo doesn't exist.

VPATH=dir

test: bar

test1:
	mkdir dir
	touch dir/foo

test2: bar

bar: foo
	echo PASS
