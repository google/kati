# TODO(go): Fix

export FOO=-x

override SHELL := PS4="cmd: " /bin/bash $${FOO}
$(info $(shell echo foo))

test:
	@echo baz
