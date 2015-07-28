unexport A

A="$${A}"
B=$(A)

test:
	echo $(B)
