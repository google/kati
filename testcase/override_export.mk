# TODO: Fix - "override export define C" is invalid "override" directive.

override export A:=PASS_A
export override B:=PASS_B
override export define C
PASS_C
endef
override export define D
PASS_D
endef

A:=FAIL_A
B:=FAIL_B
C:=FAIL_C
D:=FAIL_D

test:
	echo $$A
	echo $$B
	echo $$C
	echo $$D
