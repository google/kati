# TODO
define cmd
echo foo
echo bar
endef

test:
	@$(call cmd)
