# Ninja only supports duplicated targets for pure dependencies.
# These will both be mapped to the same target. Two rules like
# this should cause an warning (and really should cause an warning
# in make too -- this can be very confusing, and can be racy)
ifneq ($(MAKE),kati)
$(info ninja: warning: multiple rules generate a/b.)
endif

test: a/b a/./b

a/b:
	mkdir -p $(dir $@)
	echo $@

a/./b:
