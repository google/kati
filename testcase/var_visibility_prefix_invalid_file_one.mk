FOO := foo
BAR := bar
$(KATI_visibility_prefix FOO, Makefile)
$(KATI_visibility_prefix BAR, )
$(KATI_visibility_prefix BAZ, baz)

VAR0 := $(FOO)
VAR1 := $(BAR)
VAR2 := $$(BAZ)
VAR3 := $($(BAZ))

define ERROR_MSG
Makefile is not a valid file to reference variable BAZ. Line #10.
Valid file prefixes:
baz
endef

ifndef KATI
$(info $(ERROR_MSG))
endif

test:
	@:
