$(KATI_visibility_prefix FOO, foo ../foo)

ifndef KATI
$(info Makefile:1: Visibility prefix should not start with ../)
endif

test:
	@:
