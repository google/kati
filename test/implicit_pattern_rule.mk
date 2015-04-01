# Preparation: create foo.c
test1:
	touch foo.c

# foo.o should match the pattern rule below.
test2: foo.o

%.o: %.c
	echo FAIL

%o: %c
	echo FAIL

# The last one should be used.
%.o: %.c
	echo PASS
