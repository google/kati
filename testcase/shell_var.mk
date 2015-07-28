# TODO(c-ninja): Fix - ninja.sh should not use $SHELL.

$(info $(SHELL))

SHELL:=/bin/echo

$(info $(shell foo))

echo=/bin/echo
SHELL=$(echo)

$(info $(shell bar))

test:
	baz
