# TODO(c): Implement SHELL

$(info $(SHELL))

SHELL:=/bin/echo

$(info $(shell foo))

# TODO: Fix.
#echo=/bin/echo
#SHELL=$(echo)

$(info $(shell bar))

test:
	baz
