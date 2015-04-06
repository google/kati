test:
	echo $(sort foo bar lose)
	echo $(sort foo bar aaaa)
	echo $(sort foo bar lose lose foo bar bar)
	echo $(sort )
