$(KATI_visibility_prefix FOO, foo fo) # no error, different path prefixes
$(KATI_visibility_prefix Bar, bar/baz bar)

ifndef KATI
$(info Makefile:2: Visibility prefix bar is the prefix of another visibility prefix bar/baz)
endif

test:
	@:
