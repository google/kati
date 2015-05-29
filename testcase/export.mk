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

VARREF:=VAR1 VAR2
export $(VARREF)
VAR1:=PASS_VAR1
VAR2:=PASS_VAR2

test:
	echo $$FOO
	echo $$BAR
	echo $$BAZ
	echo $$X
	echo $$Y
	echo $$Z
	echo $$VAR1
	echo $$VAR2
