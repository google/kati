# TODO(all/test2): Fix

test1:
	touch a.src

test2: a.out

# This isn't in .SUFFIXES.
.src.out:
	echo $< > $@
