MAKEVER:=$(shell make --version | ruby -n0e 'puts $$_[/Make (\d)/,1]')

test: abcd

abcd:

# GNU make 3 does not prioritize the rule with a shortest stem.
ifeq ($(MAKEVER),4)
a%:
	echo FAIL
endif
abc%:
	echo PASS
ab%:
	echo FAIL
