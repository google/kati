TRUE:=foo
FALSE:=

$(if ${TRUE}, $(info PASS), $(info FAIL))
$(if ${FALSE}, $(info FAIL), $(info PASS))
$(info $(if ${TRUE}, PASS, FAIL))
# Too many arguments
$(info $(if ${FALSE}, FAIL, PASS, PASS))

test:
	echo OK

