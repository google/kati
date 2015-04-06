# Preparation: create foo.c
test1:
	touch foo.c

# foo.o should match the pattern rule below.
test2: foo.o

%.o: %.c
	echo FAIL

# This passes with GNU make 4.0 but fails with 3.81.
#%o: %c
#	echo FAIL2

# The last one should be used.
%.o: %.c
	echo PASS
