override A:=PASS_A
A:=FAIL_A

override define B
PASS_B
endef
B:=FAIL_B

override C := FAIL_C
override C := PASS_C
C := FAIL_C2

test:
	echo $(A)
	echo $(origin A)
	echo $(B)
	echo $(origin B)
	echo $(C)
	echo $(origin C)
