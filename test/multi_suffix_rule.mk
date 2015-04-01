# Preparation: create foo.c
test1:
	touch foo.c

# foo.o should match the suffix rule below.
test2: foo.o

.c.o:
	echo PASS $@ $< $^

.cc.o:
	echo FAIL
