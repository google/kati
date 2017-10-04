# TODO(go): not implemented
#
# We go into a lot more cases in deprecated_var.mk, and hope that if deprecated works, obsolete does too.

$(KATI_obsolete_var A)
$(A) $(or $(KATI),$(error A is obsolete))
