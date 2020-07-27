B = $(C)
C = $(A)
A = $(B)

foo:
	 echo $(A) > $@
