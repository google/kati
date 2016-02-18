# TODO(go): Fix

SHELL := PS4="cmd: " /bin/bash -x
$(info $(shell echo foo))

test:
	@echo baz
