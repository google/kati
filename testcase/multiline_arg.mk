SHELL:=/bin/bash

define func
$(info INFO: $(1))
echo $(1)
endef

$(info INFO2: $(call func, \
	foo))

test:
	$(call func, \
	foo)
	$(call func, \)
