# TODO(go): Implement vpath and VPATH and test them. It seems Android
# actually uses it...
# TODO(c): Not sure if C++ version passes this test.

VPATH=dir

test1:
	mkdir dir
	touch dir/foo

test2: bar

bar: foo
	echo PASS
