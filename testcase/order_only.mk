test1:
	touch -t 197101010000 foo
	touch bar

# Note order-only dependency will not appear in $^
test2: foo | bar
	echo PASS_$^

# bar is newer than foo but we should not rebuild it.
foo: | bar baz

baz:
	touch $@
