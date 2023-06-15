A := test
$(KATI_deprecated_var A B C D)

$(info Writing to an undefined deprecated variable)
B := test
ifndef KATI
$(info Makefile:5: B has been deprecated.)
endif

$(info Reading from deprecated variables - set before/after/never the deprecation func)
$(info Writing to an undefined deprecated variable)
D := $(A)$(B)$(C)
ifndef KATI
$(info Makefile:12: A has been deprecated.)
$(info Makefile:12: B has been deprecated.)
$(info Makefile:12: C has been deprecated.)
$(info Makefile:12: D has been deprecated.)
endif

$(info Writing to a reset deprecated variable)
D += test
ifndef KATI
$(info Makefile:21: D has been deprecated.)
endif

$(info Using a custom message)
$(KATI_deprecated_var E,Use X instead)
E = $(C)
ifndef KATI
$(info Makefile:28: E has been deprecated. Use X instead.)
endif

$(info Expanding a recursive variable with an embedded deprecated variable)
$(E)
ifndef KATI
$(info Makefile:34: E has been deprecated. Use X instead.)
$(info Makefile:34: C has been deprecated.)
endif

$(info All of the previous variable references have been basic SymRefs, now check VarRefs)
F = E
G := $($(F))
ifndef KATI
$(info Makefile:42: E has been deprecated. Use X instead.)
$(info Makefile:42: C has been deprecated.)
endif

$(info And check VarSubst)
G := $(C:%.o=%.c)
ifndef KATI
$(info Makefile:49: C has been deprecated.)
endif

$(info Deprecated variable used in a rule-specific variable)
test: A := $(E)
ifndef KATI
$(info Makefile:55: E has been deprecated. Use X instead.)
$(info Makefile:55: C has been deprecated.)
# A hides the global A variable, so is not considered deprecated.
endif

$(info Deprecated variable used as a macro)
A := $(call B)
ifndef KATI
$(info Makefile:63: B has been deprecated.)
$(info Makefile:63: A has been deprecated.)
endif

$(info Deprecated variable used in an ifdef)
ifdef C
endif
ifndef KATI
$(info Makefile:70: C has been deprecated.)
endif

$(info Deprecated variable used in a rule)
test:
	echo $(C)Done
ifndef KATI
$(info Makefile:78: C has been deprecated.)
endif
