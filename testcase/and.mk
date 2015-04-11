TRUE:=foo
FALSE:=
XY:=x 	y
X:=$(subst y, ,$(XY))
Y:=$(subst x, ,$(XY))

$(and ${TRUE}, $(info PASS_1))
$(and ${FALSE}, $(info FAIL_2))
# Too many arguments.
$(info $(and ${TRUE}, PASS, PASS))

$(info $(and ${TRUE}, $(X)  ))
$(info $(and ${TRUE}, $(Y)  ))
$(and ${FALSE} , $(info FAIL_3))

test:
	echo OK
