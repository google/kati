sources := foo.c bar.c baz.s ugh.h

test:
	echo cc $(filter %.c %.s,$(sources)) -o foo
