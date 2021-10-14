srcdir := .

test: foo.o xbar.o
	echo linking $@ from $<

%.o: $(srcdir)/%.c
	echo compiling $@ from $<

$(srcdir)/foo.c:
	echo source $@

xbar.c:
	echo source $@
