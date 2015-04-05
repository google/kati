foo:=$(join a b,.c .o)

# produces `a.c b.o'.
test:
	echo $(foo)

