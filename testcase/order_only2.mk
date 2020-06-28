# TODO(ninja/test2): Ninja does not believe the timestamp so this test is invalid.

test1:
	touch -t 197101010000 old1
	touch -t 197101010000 old2
	touch new

test2: old1 old2
	echo DONE

old1: | new
	echo FAIL

old2: new
	echo PASS

new:
	echo FAIL_new
