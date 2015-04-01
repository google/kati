# TODO
foo = $(realpath ./foo)
bar = $(realpath ./bar)

test: foo
	echo $(foo)
	echo $(bar)

foo:
	touch foo
