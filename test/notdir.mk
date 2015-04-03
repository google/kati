test:
	echo $(notdir foo)
	echo $(notdir foo,bar)
	echo $(notdir foo bar)
	echo $(notdir .)
	echo $(notdir /)
	echo $(notdir )
	echo $(notdir src/foo.c hacks)
	echo $(notdir hacks src/foo.c)
	echo $(notdir hacks / src/foo.c)


