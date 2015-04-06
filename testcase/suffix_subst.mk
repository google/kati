foo:= hoge.c mgoe.c

test:
	echo $(foo:.c=.o)
