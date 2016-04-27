func = $(info called with '$(1)')
test = $(call $(1),$(1))

$(call test,func)
$(call test, func)
$(call test,func )
$(call test, func )

test:
