FOO=$(shell echo SHOULD_NOT_BE_AFTER_ECHO 1>&2)

test:
	echo $(FOO)
