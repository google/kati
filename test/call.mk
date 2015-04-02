# from gyp-generated Makefile
empty :=
space := $(empty) $(empty)

replace_spaces = $(subst $(space),?,$1)
unreplace_spaces = $(subst ?,$(space),$1)
dirx = $(call unreplace_spaces,$(dir $(call replace_spaces,$1)))

test: foo
	echo $(call dirx,foo/bar)
	echo $(call dirx,foo bar/baz quux)
	echo $(call dirx,foo,bar)

foo:
	mkdir foo "foo bar"

