A = a
B = $(A)
C := $(A)
A = aa
D = b
D += b
E ?= c
E ?= d

test:
	echo $(B) $(C) $(D) $(E)
