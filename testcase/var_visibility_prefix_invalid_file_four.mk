$(KATI_visibility_prefix BAR, bar)
FOO = $(BAR) # this should be okay, since it's not evaluated
BAZ := $(FOO)

define ERROR_MSG
Makefile is not a valid file to reference variable BAR. Line #3.
Valid file prefixes:
bar
endef

ifndef KATI
$(info $(ERROR_MSG))
endif

test:
	@:
