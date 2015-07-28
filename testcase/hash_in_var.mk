$(shell mkdir -p tmp)
file = $(shell echo tmp/test\#.ext)

all: test1

test1: $(file)
	echo PASS

$(file):
	touch $(file)
