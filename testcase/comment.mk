FOO=OK  # A comment
# A multiline comment \
FOO=fail

test:
	echo $(FOO)
