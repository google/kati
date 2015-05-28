test:
	echo $(wordlist 2, 3, foo bar baz)
	echo $(wordlist 2, 4, foo bar baz)
	echo $(wordlist 4, 7, foo bar baz)
	echo $(wordlist 3, 2, foo bar baz)
	echo $(wordlist 3, 0, foo bar baz)
