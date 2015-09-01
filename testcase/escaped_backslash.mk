no_comment:=\\
# FAIL
two_backslash:=\\ foo
test:
	echo $(no_comment)
	echo $(two_backslash)
	echo \\
	echo \\ foo
	$(info echo $(no_comment))
	$(info echo $(two_backslash))
	$(info echo \\)
	$(info echo \\ foo)
