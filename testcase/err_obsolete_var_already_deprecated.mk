# TODO(go): not implemented

$(KATI_deprecated_var A)
$(KATI_obsolete_var A)$(or $(KATI),$(error Cannot call KATI_obsolete_var on already deprecated variable: A))
