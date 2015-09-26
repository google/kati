# TODO: Fix

test: xyz

xyz:: %z: %a
	echo 1 $*

xyz:: x%: a%
	echo 2 $*

ayz xya:
	echo 3 $@
