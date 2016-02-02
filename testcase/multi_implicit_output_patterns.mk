# TODO(go): Fix

all: a.h.x a.c.x a.h.z a.c.z b.h.x b.c.x b.h.z b.c.z

a.h.%:
	echo twice $@
a.c.%:
	echo twice $@

b.h.% b.c.%:
	echo once $@

b.h.z: pass

b.c.z: fail

pass:
	echo PASS

fail:
	echo FAIL
