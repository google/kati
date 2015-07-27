# TODO(ninja): Fix

test: foo

foo: bar baz
	echo $@
	echo $<
	echo $^

bar:
baz:
