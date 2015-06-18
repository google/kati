# TODO
x:=FAIL
$(foreach x,FAIL PASS,$(eval x+=$(x)))
$(info $(x))
