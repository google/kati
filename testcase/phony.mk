.PHONY: foo
	echo PASS phony foo
.PHONY: bar
.PHONY: test4

# if no foo target, but foo is .PHONY, don't warn
# "Circular baz <- foo dependency dropped.".
baz: foo
	echo baz

test1: foo bar baz
	echo PASS test1 from foo bar baz

test3:
	touch test4

test4:
	echo PASS test4

# test5 is similar with test1, but foo2 has command.
# foo2 runs once to build test5 even if it appears twice
# test5 <- foo2, test5 <- baz2 <- foo2.
.PHONY: foo2

foo2:
	echo foo2
baz2: foo2
	echo baz2

test5: foo2 bar baz2
	echo PASS test5 from foo bar baz

