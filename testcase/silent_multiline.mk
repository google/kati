define cmd
echo foo
echo bar
endef

define cmd2
echo baz
@$(call cmd)
endef

test:
	$(call cmd)
	@$(call cmd)
	$(call cmd2)
	@$(call cmd2)
