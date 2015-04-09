S:=simple
R=recursive
SE:=
RE=

foo=FOO
bar=

S+=$(foo) $(bar)
R+=$(foo) $(bar)
SE+=$(foo) $(bar)
RE+=$(foo) $(bar)
U+=$(foo) $(bar)

bar=BAR

test:
	echo "$(S)"
	echo "$(R)"
	echo "$(SE)"
	echo "$(RE)"
	echo "$(U)"
	echo "$(flavor S)"
	echo "$(flavor R)"
	echo "$(flavor SE)"
	echo "$(flavor RE)"
	echo "$(flavor U)"
