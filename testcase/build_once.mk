# expect protoc compile/link only once.
test: foo

foo: foo.o bar.o
	echo link $@ from $<

%.o: %.c FORCE_DO_CMD
	echo compile $@ from $<

.PHONY: FORCE_DO_CMD
FORCE_DO_CMD:

foo.c: | protoc

foo.c: foo.proto
	echo protoc $@ from $<

foo.proto:

bar.c: | protoc

bar.c: bar.proto
	echo protoc $@ from $<

bar.proto:

protoc: proto.o
	echo link $@ from $<

proto.c:
