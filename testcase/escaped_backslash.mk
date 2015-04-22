no_comment:=\\
# FAIL
two_backslash:=\\ foo
test:
	echo $(no_comment)
	echo $(two_backslash)
	echo \\
	echo \\ foo
