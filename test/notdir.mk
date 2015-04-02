test:
	echo $(notdir foo)
	echo $(notdir foo,bar)
	echo $(notdir foo bar)
	echo $(notdir .)
	echo $(notdir /)
	echo $(notdir )


