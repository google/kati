# TODO(go): not implemented

$(KATI_obsolete_var A)
$(KATI_deprecated_var A)$(or $(KATI),$(error Cannot call KATI_deprecated_var on already obsolete variable: A))
