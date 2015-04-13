# http://www.gnu.org/software/make/manual/make.html#Splitting-Recipe-Lines
# TODO: Fix.
test1:
	echo no\
space
	#echo no\
	#space
	echo one \
	space
	echo one\
	 space

test2:
	for d in foo bar; do \
	  echo $$d ; done

define cmd3
echo foo
echo bar
endef

test3:
	$(cmd3)

define cmd4
echo foo ; \
echo bar
endef

test4:
	$(cmd4)

test5:
	echo foo \
	$$empty bar

# TODO: Fix.
#test6:
#	echo foo\
#	$${empty}bar

