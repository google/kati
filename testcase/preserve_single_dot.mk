# TODO(ninja): Fix - ninja normalize a/./b to a, and mkdir a

test: a/./b ./x

a/./b:
	echo $@

././x:
	echo $@
