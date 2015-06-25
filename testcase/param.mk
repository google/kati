# TODO(c): Fix

1:=foo
$(info $(1) is foo)

define param
$(eval 1:=bar) param1-1=$(1) $(call param2,$(1))
endef

define param2
param2-1=$(1)
endef

test:
	@echo call param $(call param,baz)
	@echo 1=$(1)
