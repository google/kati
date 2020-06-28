# TODO(c-ninja): $(shell) in another make expression is not supported.

test:
	echo $(strip $(shell dirname $$(pwd)))
