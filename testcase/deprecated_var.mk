# TODO(go): not implemented


A := test
$(KATI_deprecated_var A B C D)

# Writing to an undefined deprecated variable
B := test
ifndef KATI
$(info Makefile:8: B has been deprecated.)
endif

# Reading from deprecated variables (set before/after/never the deprecation func)
# Writing to an undefined deprecated variable
D := $(A)$(B)$(C)
ifndef KATI
$(info Makefile:15: A has been deprecated.)
$(info Makefile:15: B has been deprecated.)
$(info Makefile:15: C has been deprecated.)
$(info Makefile:15: D has been deprecated.)
endif

# Writing to a reset deprecated variable
D += test
ifndef KATI
$(info Makefile:24: D has been deprecated.)
endif

# Using a custom message
$(KATI_deprecated_var E,Use X instead)
E = $(C)
ifndef KATI
$(info Makefile:31: E has been deprecated. Use X instead.)
endif

# Expanding a recursive variable with an embedded deprecated variable
$(E)
ifndef KATI
$(info Makefile:37: E has been deprecated. Use X instead.)
$(info Makefile:37: C has been deprecated.)
endif

# All of the previous variable references have been basic SymRefs, now check VarRefs
F = E
G := $($(F))
ifndef KATI
$(info Makefile:45: E has been deprecated. Use X instead.)
$(info Makefile:45: C has been deprecated.)
endif

# And check VarSubst
G := $(C:%.o=%.c)
ifndef KATI
$(info Makefile:52: C has been deprecated.)
endif

# Deprecated variable used in a rule-specific variable
test: A := $(E)
ifndef KATI
$(info Makefile:58: E has been deprecated. Use X instead.)
$(info Makefile:58: C has been deprecated.)
# A hides the global A variable, so is not considered deprecated.
endif

# Deprecated variable used in a rule
test:
	echo $(C)Done
ifndef KATI
$(info Makefile:67: C has been deprecated.)
endif
