# TODO
# http://www.gnu.org/software/make/manual/make.html#Multi_002dLine
# see also define.mk

override define two-lines
echo foo
echo $(bar)
endef

bar = xxx

test:
	echo BEGIN $(two-lines) END
