# TODO: Maybe unecessary

test1:
	# foo
	echo PASS

test2:
	# foo  \
	echo PASS

test3: $(shell echo foo #)

test4:
	echo $(shell echo OK #)
