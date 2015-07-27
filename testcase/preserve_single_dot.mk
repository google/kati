# TODO(ninja): Fix

test: a/./b ./x

a/./b:
	echo $@

././x:
	echo $@
