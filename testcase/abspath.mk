foo = $(abspath ./foo bar/../foo bar//..//foo / /usr)

test:
	echo $(foo)
