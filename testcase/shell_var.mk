$(info $(SHELL))

override SHELL:=/bin/echo

$(info $(shell foo))

echo=/bin/echo
override SHELL=$(echo)

$(info $(shell bar))

test:
	baz
