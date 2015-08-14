define q
$$(info define q)
$$(info $$(subst $$$$,$$$$$$$$,$$q))
$$(info endef)
$$(info $$$$(eval $$$$q))
endef
$(eval $q)
