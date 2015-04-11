TRUE:=foo
FALSE:=

$(if ${TRUE}, $(info PASS1))
$(if ${FALSE}, $(info FAIL1))
$(if ${TRUE}, $(info PASS2), $(info FAIL2))
$(if ${FALSE}, $(info FAIL3), $(info PASS3))
$(info $(if ${TRUE}, PASS4, FAIL4))
# Too many arguments
$(info $(if ${FALSE}, FAIL5, PASS5, PASS6))
$(info $(if ${FALSE} , FAIL6, PASS7))

test:
	echo OK

