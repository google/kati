TRUE:=foo
FALSE:=
XY:=x 	y
X:=$(subst y, ,$(XY))
Y:=$(subst x, ,$(XY))

$(or ${FALSE}, $(info PASS_1))
# expect "foo"
$(info $(or ${TRUE}, $(info FAIL_2)))
# Too many arguments.
$(info $(or ${FALSE}, PASS, PASS))

$(info $(or ${FALSE}, $(X)  ))
$(info $(or ${FALSE}, $(Y)  ))
$(info $(or ${FALSE} , PASS, PASS))

test:
	echo OK
