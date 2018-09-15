# TODO(go): not implemented


A := test
$(KATI_deprecated_var A B C D)

$(info Writing to an undefined deprecated variable)
B := test
ifndef KATI
$(info Makefile:8: B has been deprecated.)
endif

$(info Reading from deprecated variables - set before/after/never the deprecation func)
$(info Writing to an undefined deprecated variable)
D := $(A)$(B)$(C)
ifndef KATI
$(info Makefile:15: A has been deprecated.)
$(info Makefile:15: B has been deprecated.)
$(info Makefile:15: C has been deprecated.)
$(info Makefile:15: D has been deprecated.)
endif

$(info Writing to a reset deprecated variable)
D += test
ifndef KATI
$(info Makefile:24: D has been deprecated.)
endif

$(info Using a custom message)
$(KATI_deprecated_var E,Use X instead)
E = $(C)
ifndef KATI
$(info Makefile:31: E has been deprecated. Use X instead.)
endif

$(info Expanding a recursive variable with an embedded deprecated variable)
$(E)
ifndef KATI
$(info Makefile:37: E has been deprecated. Use X instead.)
$(info Makefile:37: C has been deprecated.)
endif

$(info All of the previous variable references have been basic SymRefs, now check VarRefs)
F = E
G := $($(F))
ifndef KATI
$(info Makefile:45: E has been deprecated. Use X instead.)
$(info Makefile:45: C has been deprecated.)
endif

$(info And check VarSubst)
G := $(C:%.o=%.c)
ifndef KATI
$(info Makefile:52: C has been deprecated.)
endif

$(info Deprecated variable used in a rule-specific variable)
test: A := $(E)
ifndef KATI
$(info Makefile:58: E has been deprecated. Use X instead.)
$(info Makefile:58: C has been deprecated.)
# A hides the global A variable, so is not considered deprecated.
endif

$(info Deprecated variable used as a macro)
A := $(call B)
ifndef KATI
$(info Makefile:66: B has been deprecated.)
$(info Makefile:66: A has been deprecated.)
endif

$(info Deprecated variable used in an ifdef)
ifdef C
endif
ifndef KATI
$(info Makefile:73: C has been deprecated.)
endif

$(info Deprecated variable used in a rule)
test:
	echo $(C)Done
ifndef KATI
$(info Makefile:81: C has been deprecated.)
endif
