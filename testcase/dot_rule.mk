# Rules start with dots cannot be the first rule.
.foo:
	echo FAIL

test:
	echo PASS
