# TODO: Fix

override export A:=PASS_A
export override B:=PASS_B
export override define C
PASS_C
endef

A:=FAIL_A
B:=FAIL_B
C:=FAIL_C

test:
	echo $$A
	echo $$B
	echo $$C
