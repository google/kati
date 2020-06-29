# TODO(go): Fix

MAKEVER:=$(shell make --version | grep "Make [0-9]" | sed -E 's/.*Make ([0-9]).*/\1/')

all: a.h.x a.c.x a.h.z a.c.z b.h.x b.c.x b.h.z b.c.z

a.h.%:
	echo twice $@
a.c.%:
	echo twice $@

b.h.% b.c.%:
	echo once $@

b.h.z: pass

# GNU make 4 invokes this rule.
ifeq ($(MAKEVER,3))
b.c.z: fail
endif

pass:
	echo PASS

fail:
	echo FAIL
