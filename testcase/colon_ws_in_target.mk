# TODO(c|go-ninja): Fix
# go-ninja: wrong escape a:b vs a\:b

test: a\ b
	echo $@ / $<

a\ b: a\:b
	echo $@ / $<

a\\\:b:
	echo a\\\:b $@
