export FOO = PASS_FOO
BAR := PASS_BAR
export BAR
export X Y Z
X := PASS_X
Y := PASS_Y
Z := PASS_Z

export BAZ = FAIL
unexport BAZ

unexport Y
export Y X

# GNU make 3 and 4 behave differently for this, but it must not mess
# up FOO, BAR, X, Y, and Z.
export FOO BAR X Y Z := FAIL

test:
	echo $$FOO
	echo $$BAR
	echo $$BAZ
	echo $$X
	echo $$Y
	echo $$Z
