# Preparation: create foo.c bar.c baz.cc
test1:
	touch foo.c bar.c baz.cc

test2: foo.o bar.o baz.o

# The right choice for foo.o
foo.o: %.o: %.c
	echo PASS_foo

# The right choice for bar.o
%.o: %.c
	echo PASS_bar

# This rule should be dominated by other rules
.c.o:
	echo FAIL

# The right choice for baz.o
.cc.o:
	echo PASS_baz
