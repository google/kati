# http://www.gnu.org/software/make/manual/make.html#Multi_002dLine
# see also define.mk

override define two-lines
echo foo
echo $(bar)
endef

bar = xxx

override CC := gcc
override  AS = as
override  define three-lines
echo 1
echo 2
echo 3
endef
override	define  four-lines
echo I
echo II
echo III
echo IV
endef

test:
	echo CC=$(CC) $(flavor CC)
	echo AS=$(AS) $(flavor AS)
	echo two BEGIN $(two-lines) END $(flavor two-lines)
	echo three BEGIN $(three-lines) END $(flavor three-lines)
	echo four BEGIN $(four-lines) END $(flavor four-lines)
