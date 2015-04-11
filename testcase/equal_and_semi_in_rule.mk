test: foo bar

foo: A=echo ; echo PASS
foo:
	echo $(A)

bar: ; echo PASS=PASS
