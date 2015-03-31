prefix = M
pattern = ${prefix}*
files = $(wildcard $(pattern))

# expect Makefile, since runtest.rb put this as Makefile in new dir.
test:
	echo $(files)
