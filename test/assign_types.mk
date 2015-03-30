A = a
B = $(A)
C := $(A)
A = aa

test:
	echo $(B) $(C)
