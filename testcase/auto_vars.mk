test: foo bar

foo: baz
	echo $<

bar:
	echo $<

baz:
