FOO := foo
BAR := bar
PREFIX := pone/ptwo

$(KATI_visibility_prefix FOO, pone/ptwo baz)
$(KATI_visibility_prefix FOO, $(PREFIX) baz)

$(KATI_visibility_prefix BAR, pone/ptwo baz)
$(KATI_visibility_prefix BAR, baz $(PREFIX))

ifndef KATI
$(info Visibility prefix conflict on variable: BAR)
endif

test:
	@:
