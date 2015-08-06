# TODO(ninja): This test is only for ckati. ninja: fix $(sort $(shell $(1)))
# go: implement generic builtin find
# ninja: $(sort $(shell "find .")) becomes "$( .) find"

define run_find
@echo $$ '$(strip $(1))'
@echo $(sort $(shell $(1)))
endef

test1:
	$(call run_find, find .)
