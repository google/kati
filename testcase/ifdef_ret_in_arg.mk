define x
a
b
endef
$(x):=PASS
ifdef $(x)
$(info $($(x)))
endif
