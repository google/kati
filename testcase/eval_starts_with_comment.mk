.PHONY: test

define _rule
# comment
test:
	echo PASS
endef

$(eval $(_rule))
