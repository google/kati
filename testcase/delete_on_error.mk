# TODO: Fix for non-ninja mode.

.DELETE_ON_ERROR:

test: file

file:
	touch $@
	false
