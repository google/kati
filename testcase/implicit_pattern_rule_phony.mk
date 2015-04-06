test: foo.x

test2: all

.PHONY: FORCE
FORCE:

all: foo.y
	echo $@ from $<

%.y: %.x FORCE
	echo $@ from $<

foo.x:
	touch foo.x
