test: foo.c foo.h

foo.c foo.h:
	touch $@

test2: foo

CFLAGS=-O
foo: foo.o
	echo cc $(CFLAGS) -o $@ $<
foo.o: CFLAGS=-g
foo.o: foo.h

%.o: %.c
	echo cc $(CFLAGS) -o $@ -c $<
