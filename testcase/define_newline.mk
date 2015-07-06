define newline


endef

$(info This should have$(newline)two lines)

test:
	echo OK
