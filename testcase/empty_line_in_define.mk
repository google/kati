define foo
echo foo

endef

define bar

echo bar

endef

define baz
echo baz

echo baz
endef

test:
	$(foo) $(foo)
	$(bar) $(bar)
	$(baz) $(baz)
