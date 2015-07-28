# TODO(c-ninja): Fix - should emit phony rule for bar baz

test: foo

foo: bar baz
	echo $@
	echo $<
	echo $^

bar:
baz:
