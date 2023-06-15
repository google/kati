define var
VAR:=1
endef

$(call var)

eq_one:==1
$(eq_one):
	echo FAIL
