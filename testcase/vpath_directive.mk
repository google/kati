# TODO(go): Implement vpath. It seems Android actually uses it...
# TODO(c): bar is built even if foo doesn't exist.

vpath dir %.c

test: bar

test1:
	mkdir dir
	touch dir/foo.c

test2: bar

bar: foo.c
	echo PASS
