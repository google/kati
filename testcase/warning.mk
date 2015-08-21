$(warning foo)

define baz
b
a
z
endef

test:
	$(warning bar'""')
	$(warning $(baz))
	echo PASS
