test: override

A=PASS
# The behavior for this depends on the version of GNU make. It looks
# like old GNU make has a bug here.
# override : A=PASS_2
override :
	echo $(A)
