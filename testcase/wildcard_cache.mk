# maybe, make has wildcard cache at startup time?
files = $(wildcard *,*)

# if make starts without foo,bar, expect foo,bar, but it will be empty.
test: foo,bar
	echo $(files)

# first $(files) will be empty since no foo,bar exists.
# second $(files) expects foo, but empty.
foo,bar:
	echo $(files)
	touch foo,bar
	echo $(files)
