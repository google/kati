# TODO(ninja): Fix?

test: self loop not_circular1 not_circular2
	echo PASS

self: self
	echo $@

loop: loop1
	echo $@

loop1: loop2
	echo $@

loop2: loop
	echo $@

not_circular1: Makefile

not_circular2: Makefile
