# TODO(c): Implement vpath.

vpath %.c dir

test: bar

test1:
	mkdir dir
	touch dir/foo.c

test2: bar

bar: foo.c
	echo PASS
