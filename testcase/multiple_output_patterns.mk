test: foo.o

foo.o: %.o: %.c

foo.o: foo.%: bar.%

foo.o: foo.h
	cp $< $@

foo.h foo.c bar.o:
	touch $@
