define foo
 $(eval X:=) \
 $(eval X:=) \
 $(warning foo)
endef

$(call foo)

test:
	echo FOO
