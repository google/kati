# TODO(hamaji): Fix.

test1:
	touch foo
	touch bar

test2: foo
	echo PASS

foo: | bar
	touch $@
