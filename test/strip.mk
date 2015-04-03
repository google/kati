XY:=x 	y
X:=$(subst y, ,$(XY))
Y:=$(subst x, ,$(XY))

test:
	echo $(X)
	echo $(Y)
	echo $(strip $(X))
	echo $(strip $(Y))
	echo $(strip $(Y),$(X))

# TODO: Hard to tell why. Fix this.
#test2:
#	echo $(strip $(X),$(Y))
