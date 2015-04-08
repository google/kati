# TODO
srcdir := .

test: foo.o
	echo linking $@ from $<

foo.o: $(srcdir)/foo.c
	echo compiling $@ from $<

$(srcdir)/foo.c:
	echo source $@
