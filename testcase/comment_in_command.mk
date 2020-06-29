MAKEVER:=$(shell make --version | grep "Make [0-9]" | sed -E 's/.*Make ([0-9]).*/\1/')

test1:
	# foo
	echo PASS

test2: make$(MAKEVER)

make4:
	# foo  \
echo PASS

make3:
	# foo  \
	echo PASS

test3: $(shell echo foo #)

test4:
	echo $(shell echo OK # FAIL \
	FAIL2)

test5:
	echo $(shell echo $$(echo PASS))

foo:
	echo OK
