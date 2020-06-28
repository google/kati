# TODO(ninja-genall)
# TODO(ninja/test2): This is tough to fix with ninja. Ninja normalizes
# target names while make does not.

test1:
	mkdir a b

test2: a/b a/../a/b a/./b b/a b/../b/a b/./a

a/%:
	touch $@

b/%:
	echo $@
