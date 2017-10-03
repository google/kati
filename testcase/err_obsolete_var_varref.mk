# TODO(go): not implemented

$(KATI_obsolete_var A)
B := A
$($(B)) $(or $(KATI),$(error A is obsolete))
