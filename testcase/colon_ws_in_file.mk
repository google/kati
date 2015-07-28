# TODO(c|go-ninja): Fix
# go-ninja: wrong escape a:b vs a\:b

test: a\ b a\:b

a%:
	echo $@
