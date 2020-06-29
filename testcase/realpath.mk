$(shell touch ../foo)

foo = $(realpath ../foo)
bar = $(realpath ../bar)
foofoo = $(realpath ../foo ../foo)
foobar = $(realpath ../foo ../bar)

test:
	echo $(foo)
	echo $(bar)
	echo $(foofoo)
	echo $(foobar)
