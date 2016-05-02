# TODO(go): Fix

MAKEVER:=$(shell make --version | ruby -n0e 'puts $$_[/Make (\d)/,1]')

ifeq ($(MAKEVER),4)

# GNU make 4 escapes $(SHELL).
test:
	echo test skipped

else

export FOO=-x

override SHELL := PS4="cmd: " /bin/bash $${FOO}
$(info $(shell echo foo))

test:
	@echo baz

endif
