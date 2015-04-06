foo=foo.d missing.d
empty=

test1:
	echo "foo: bar" > foo.d

test2: foo

bar:
	echo OK

-include $(foo)
include $(empty)
