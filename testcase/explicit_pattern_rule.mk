# Preparation: create foo.c
test1:
	touch foo.c

# foo.o should match the pattern rule below.
test2: foo.o

foo.o: %.o: %.c
	echo PASS
