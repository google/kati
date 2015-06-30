override A:=PASS_A
A:=FAIL_A

override define B
PASS_B
endef
B:=FAIL_B

test:
	echo $(A)
	echo $(origin A)
	echo $(B)
	echo $(origin B)
