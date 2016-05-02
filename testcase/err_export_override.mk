# TODO(c): Fix - "override export define A" is invalid "override" directive.

# GNU make 4 accepts this syntax. Note kati doesn't agree with make 4
# either.
MAKEVER:=$(shell make --version | ruby -n0e 'puts $$_[/Make (\d)/,1]')
ifeq ($(MAKE)$(MAKEVER),make4)
$(error test skipped)
endif

export override define A
PASS_A
endef
