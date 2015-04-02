foo=FOO
C ?= $(foo) $(bar)

test:
	echo "$(C)"

bar=BAR

