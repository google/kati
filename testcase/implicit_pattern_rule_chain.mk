test: foo

foo: foo.o
	echo link foo

%.o: %.c
	echo compile from $< to $@

foo.c: genc
	echo generate $@

.PHONY: genc
genc:
