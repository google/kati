files = $(wildcard *,*)

test:
	echo $(files)

test2: foo,bar
	echo $(files)

foo,bar:
	touch foo,bar
