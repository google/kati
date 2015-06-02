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

empty:=#
g := FAIL
h := $(empty) PASS

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
 $(eval g := $(h))
 echo _$(g)_
endef

a.x=X
y:=Y
$(foreach tag,x,$(eval a.$(tag)+=$(y)))
y:=Z
$(foreach tag,x,$(eval a.$(tag)+=$(y)))
$(info $(a.x))

test:
	$(call evaltest)
