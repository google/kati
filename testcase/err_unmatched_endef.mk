define test1
# Typo below, endif instead of endef
endif
define test2
endef

foo:
	echo FAIL

