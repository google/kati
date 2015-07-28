XY:=x 	y
X:=$(subst y, ,$(XY))
Y:=$(subst x, ,$(XY))

define func
foo
bar
endef

test:
	echo $(X)
	echo $(Y)
	echo $(strip $(X))
	echo $(strip $(Y))
	echo $(strip $(Y),$(X))
	echo $(strip $(XY))
	$(info $(strip $(call func)))

test2:
	echo $(strip $(X),$(Y))
