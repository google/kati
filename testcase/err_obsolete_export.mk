# TODO(go): not implemented

export A := ok

$(KATI_obsolete_export Message)

export B := fail $(or $(KATI),$(error B: export is obsolete. Message))
