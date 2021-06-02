foo := $(subst a,$,bab)

test:
	echo $(foo)
