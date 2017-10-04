# TODO(go): not implemented

$(KATI_obsolete_var A)
$(A:%.o=%.c) $(or $(KATI),$(error A is obsolete))
