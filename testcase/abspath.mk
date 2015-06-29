foo = $(abspath ./foo bar/../foo bar//..//foo / /usr)
bar = $(abspath .. ./. ./ /aa/.. a///)

test:
	echo $(foo)
	echo $(bar)
