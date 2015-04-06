# https://www.gnu.org/software/make/manual/html_node/Target_002dspecific.html

CFLAGS = -O

test: prog

prog: CFLAGS = -g
prog : prog.o
	echo prog $(CFLAGS)

prog.o : prog.c
	echo cc $(CFLAGS) -o prog.o -c prog.c

prog.c:
	touch prog.c


