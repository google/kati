define outer
  define inner
PASS
   endef
  define inner_fail
FAIL
    endef
endef

# Prefixed defines don't increase the nest level.
define outer_override
override define inner2
export define inner3
endef

A := $(inner_fail)
$(eval $(outer))

foo:
	echo $(A)
	echo $(inner)
