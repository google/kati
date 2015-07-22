# TODO(c): Fix

test: a\ b
	echo $@ / $<

a\ b: a\:b
	echo $@ / $<

a\\\:b:
	echo a\\\:b $@
