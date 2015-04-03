test:
	echo $(findstring a, a b c)
	echo $(findstring b, a b c)
	echo $(findstring b c, a b c)
	echo $(findstring a, b c)
	echo $(findstring a, b c, a)
