# TODO(go): Fix

test: PASS_FAIL PASS2_FAIL2 FAIL3.FAIL4

%_FAIL:
	echo $*

PASS2_FAIL2: %_FAIL2:
	echo $*

FAIL3.FAIL4:
	echo $(or $*,PASS3)
