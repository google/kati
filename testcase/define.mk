# http://www.gnu.org/software/make/manual/make.html#Multi_002dLine
# Note: in make 4.x
# define name =
# ...
# endef
#
# but in make 3.x
# define name
# ...
# endef
# i.e. no = needed after name.
# make 3.x defines "name =" for make 4.x example.
# TODO: should we provide flag to specify gnu make version?
# note: in make 4.x, there is `undefine`.

define two-lines
echo foo
echo $(bar)
endef

bar = xxx

test:
	echo BEGIN $(two-lines) END
