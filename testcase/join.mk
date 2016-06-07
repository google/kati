foo:=$(join a b,.c .o)

# produces `a.c b.o'.
test:
	echo $(foo)
	echo $(join a b c, 0 1)
	echo $(join a b, 0 1 2)
