foo = $(subst /rkati,,$(subst /kati,,$(subst /make,,$(abspath ./foo bar/../foo bar//..//foo / /usr))))
bar = $(subst /rkati,,$(subst /kati,,$(subst /make,,$(abspath .. ./. ./ /aa/.. a///))))

test:
	echo $(foo)
	echo $(bar)
