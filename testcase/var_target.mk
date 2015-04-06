FOO=BAR
$(FOO)=BAZ

test:
	echo $(BAR)
