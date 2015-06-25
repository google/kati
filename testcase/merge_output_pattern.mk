test: foo.o

foo.o: %.o: %.c

foo.o: foo.h
	echo $^
	cp $< $@

foo.h foo.c:
	touch $@
