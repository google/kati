# Preparation: create foo.c
test1:
	touch foo.c exist

# foo.o should match the suffix rule below.
test2: foo.o

%.o: %.c not_exist
	echo FAIL

%.o: %.c exist
	echo PASS $@ $< $^

%.o: %.c not_exist
	echo FAIL

%.o: %.cc
	echo FAIL
