foo = $(realpath ./foo)
bar = $(realpath ./bar)
foofoo = $(realpath ./foo ./foo)
foobar = $(realpath ./foo ./bar)

test: foo
	echo $(foo)
	echo $(bar)
	echo $(foofoo)
	echo $(foobar)

foo:
	touch foo
