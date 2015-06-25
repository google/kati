test1:
	touch bar baz

test2: foo

foo: bar
foo: baz
foo:
	echo $< $^
