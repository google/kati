# TODO(go-ninja): Fix - ninja normalize a/./b to a/b.

test: a/./b ./x

a/./b:
	echo $@
	mkdir -p a # for ninja.

././x:
	echo $@
