test: foo.o

foo.o: %.o: %.c

foo.o: foo.h
	cp $< $@

foo.h foo.c:
	touch $@
