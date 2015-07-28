# TODO(go-ninja): should not emit "build a/./b: phony"
# Unlike ninja_normalized_path.mk, this passes with the current
# implementation because we remove a/./b. See EmitNode in ninja.cc.

test1:
	mkdir a

test2: a/b a/./b

a/b:
	echo $@

a/./b:
