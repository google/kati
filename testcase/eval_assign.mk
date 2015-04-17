bar := FAIL
pb := prog: bar
$(pb) := PASS
define evaltest
 $(eval foo := PASS)
 $(eval bar := $$(foo))
 echo $(bar)
 $(eval prog: bar := PASS)
 echo $($(pb))
endef

test:
	$(call evaltest)

