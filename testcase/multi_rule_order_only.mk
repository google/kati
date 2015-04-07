test: foo.c

foo.c:
	touch foo.c

OBJS := foo.o

$(OBJS): | bar.a 

$(OBJS): CFLAGS:=-g

%.o: %.c FORCE_DO_CMD
	echo compile $@ from $<

.PHONY: FORCE_DO_CMD
FORCE_DO_CMD:

bar.a:
	echo archive $@

foo.a: $(OBJS)
	echo archive $@

test2: foo.a



