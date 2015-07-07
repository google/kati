# http://www.gnu.org/software/make/manual/make.html#Syntax-of-Functions
comma:= ,
empty:=
space:= $(empty) $(empty)
foo:= a b c
bar:= $(subst $(space),$(comma),$(foo))
# bar is now `a,b,c'

test:
	echo $(bar)
	echo $(subst ,repl,str)
