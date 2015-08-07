define inner
{$(1)|$(origin 1),$(2)|$(origin 2)}
endef

define macro
$(call inner,$(1)) \
$(call inner,test2) \
$(call inner,test3,) \
$(call inner,test4,macro) \
$(call inner)
endef

2=global

test:
	@echo "$(call macro,test1)"
	@echo "$(call macro)"
