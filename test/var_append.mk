S:=simple
R=recursive

foo=FOO
bar=

S+=$(foo) $(bar)
R+=$(foo) $(bar)

bar=BAR

test:
	echo "$(S)"
	echo "$(R)"
	echo "$(flavor S)"
	echo "$(flavor R)"
