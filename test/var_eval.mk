# TODO

var1 = $($(bar))
var2 = $$(bar)
var3 := $($(bar))
var4 := $$(bar)

# expects
#  foo
#  $bar
#
#  $bar
test:
	echo '$(var1)'
	echo '$(var2)'
	echo '$(var3)'
	echo '$(var4)'

bar = foo
foo = foo

