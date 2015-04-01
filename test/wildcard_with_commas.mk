files = $(wildcard *,*)

# test expectes empty, since no *,* found.
test:
	echo $(files)
	touch foo,bar

# when foo,bar doesn't exit, "make test2" report empty.
# next "make test2" reports "foo,bar".
test2: foo,bar
	echo $(files)

foo,bar:
	touch foo,bar
