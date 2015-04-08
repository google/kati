srcdir := .

test: foo.o bar.o
	echo linking $@ from $<

foo.o: $(srcdir)/foo.c
	echo compiling $@ from $<
bar.o: $(srcdir)/bar.c
	echo compiling $@ from $<

$(srcdir)/foo.c:
	echo source $@

bar.c:
	echo source $@
