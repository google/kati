# TODO(go): Fix

MAKEVER:=$(shell make --version | ruby -n0e 'puts $$_[/Make (\d)/,1]')

# GNU make 3.82 has this feature though.
ifeq ($(MAKEVER),3)

test:
	echo test skipped

else

$(info $(shell echo foo))
override SHELL := echo
$(info $(shell echo bar))
.POSIX:
$(info $(shell echo baz))
test:
	foobar

endif
