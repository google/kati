bar := FAIL
pf := prog: baz
$(pf) := PASS

moge := PASS
hoge := $$(moge)

a := FAIL
b := c
c := PASS

d := FAIL
e := $$f
f := PASS

define evaltest
 $(eval foo := PASS)
 $(eval bar := $$(foo))
 echo $(bar)
 $(eval prog: baz := FAIL)
 echo $($(pf))
 $(eval fuga := $(hoge))
 echo $(fuga)
 $(eval a := $($(b)))
 echo $(a)
 $(eval d := $(e))
 echo $(d)
endef

test:
	$(call evaltest)

