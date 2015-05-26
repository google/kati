# TODO: Fix

export FOO = PASS
BAR := PASS
export BAR

export BAZ = FAIL
unexport BAZ

test:
	echo $$FOO
	echo $$BAR
	echo $$BAZ
