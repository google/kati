test:
	echo $(word 2, foo bar baz)
	echo $(word 2, )
	echo $(word 4, foo bar baz)
	echo $(word 1, foo,bar baz)
	echo $(word 2, foo,bar baz)
	echo $(word 2, foo, bar baz)
