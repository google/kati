bar := FAIL
pb := prog: bar
$(pb) := PASS

moge := PASS
hoge := $$(moge)

define evaltest
 $(eval foo := PASS)
 $(eval bar := $$(foo))
 echo $(bar)
 $(eval prog: bar := PASS)
 echo $($(pb))
 $(eval fuga := $(hoge))
 echo $(fuga)
endef

test:
	$(call evaltest)

