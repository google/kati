# http://www.gnu.org/software/make/manual/make.html#Syntax-of-Functions
,:= ,
empty:=
space:= $(empty) $(empty)
foo:= a b c
bar:= $(subst $(space),$,,$(foo))
# bar is now `,abc'
# space in `,$(foo)' replaced with `$', which will be empty

test:
	echo $(bar)
