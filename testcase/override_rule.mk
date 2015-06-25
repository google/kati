# TODO(go): Fix

test: override

A=OK
override : A=PASS
override :
	echo $(A)
