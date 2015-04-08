srcdir := .

test: foo.o bar.o
	echo linking $@ from $<

%.o: $(srcdir)/%.c
	echo compiling $@ from $<

$(srcdir)/foo.c:
	echo source $@

bar.c:
	echo source $@
