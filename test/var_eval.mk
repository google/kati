var1 = $($(bar))
var2 = $$(bar)
var3 := $($(bar))
var4 := $$(bar)

D=$$
O=(
C=)

# expects
#  foo
#  $(bar)
#
#  $(bar)
#  $(bar)
test:
	echo '$(var1)'
	echo '$(var2)'
	echo '$(var3)'
	echo '$(var4)'
	echo '$D$Obar$C'

bar = foo
foo = foo

