# expect protoc compile/link only once.

# Caveat: this test relies on Make and Ninja updating 
# the targets in the same order. Make's target update
# order is depth first, Ninja's is the first lexically
# updatable target. Caution should be taken as Kati reorders
# targets.
test: foo

foo: foo.o xbar.o
	echo link $@ from $<

%.o: %.c FORCE_DO_CMD
	echo compile $@ from $<

.PHONY: FORCE_DO_CMD
FORCE_DO_CMD:

foo.c: | protoc

foo.c: foo.proto
	echo protoc $@ from $<

foo.proto:

xbar.c: | protoc

xbar.c: xbar.proto
	echo protoc $@ from $<

xbar.proto:

protoc: proto.o
	echo link $@ from $<

proto.c:
