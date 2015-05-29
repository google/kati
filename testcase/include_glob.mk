test1:
	echo "foo: bar" > foo.d

test2: foo

bar:
	echo OK

 -include *.d
