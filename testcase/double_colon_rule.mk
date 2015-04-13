test::
	echo FOO
test::
	echo BAR

test:: A=B

# Merge a double colon rule with target specific variable is OK.
test: A=B
