# TODO(c): Fix

x=FAIL
$(foreach x,FAIL PASS,$(eval x+=$(x)))
# x will leak if assigned.
$(info $(x))
$(info $(flavor x))
$(info $(origin x))

x=PASS
$(foreach x,FAIL,)
# x won't leak
$(info $(x))
$(info $(flavor x))
$(info $(origin x))
