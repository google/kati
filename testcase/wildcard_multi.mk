files = $(wildcard P* M*)

test1:
	touch PASS

test2:
	echo $(files)
