test: foo
	echo FAIL

test: bar
	echo PASS_test

foo:
	echo FAIL_foo

foo:
	echo PASS_foo

bar:
	echo PASS_bar
