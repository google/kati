# TODO(go): Fix

export FOO=-x

SHELL := PS4="cmd: " /bin/bash $${FOO}
$(info $(shell echo foo))

test:
	@echo baz
