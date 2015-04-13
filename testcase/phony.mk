# TODO: Fix

.PHONY: foo
	echo PASS
.PHONY: bar
.PHONY: test4

test1: foo bar
	echo PASS

# Actually, you can use .PHONY!
test2: .PHONY

test3:
	touch test4

test4:
	echo PASS
