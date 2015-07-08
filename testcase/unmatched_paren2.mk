foo = FOO
bar = BAR
dp := $$(
$(dp)foo := PASS_UNMATCHED
FOO1BAR := PASS_MATCHED
baz = 0$($(foo)1$(bar)2
# baz will be 0PASS_UNMATCHED, 1$(bar)2 will be discarded??
baz2 = 0$($(foo)1$(bar))2
# baz2 will be 0PASS_MATCHED2.

test:
	echo "$(baz)"
	echo "$(baz2)"
