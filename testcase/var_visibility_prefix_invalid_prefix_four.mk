$(KATI_visibility_prefix FOO, foo/)

ifndef KATI
$(info Makefile:1: Visibility prefix foo/ is not normalized. Normalized prefix: foo)
endif

test:
	@:
