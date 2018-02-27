# TODO(go): not implemented

A := 1
B := 2
export A B

$(KATI_deprecate_export Message)

export C := ok
unexport B

ifndef KATI
$(info Makefile:9: C: export has been deprecated. Message.)
$(info Makefile:10: B: unexport has been deprecated. Message.)
endif

test:
	echo $$(A)
	echo $$(B)
	echo $$(C)
	echo Done
