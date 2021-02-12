not_traced := no
simple_immediate := \
			a \
			b \
			c
compound_immediate := \
			$(one) \
			d
simple_deferred = e
compound_deferred_1 = $(simple_immediate)
compound_deferred_2 = $(simple_deferred)
overwritten := f
overwritten := g
appened := h
appended += i
eval_macro = evaled := j
$(eval $(eval_macro))
multiple_1 := k
multiple_2 := l

# Standard make doesn't have KATI_variable_location, so the non-kati version
# prints the expected value.
#
# $(1) variable name
# $(2) expected location
define print-location
$(info KATI_variable_location: $(if $(KATI),$(KATI_variable_location $(1)),$(strip $(2))))
endef

$(call print-location, undefined_variable, <unknown>:0)
$(call print-location, not_traced, Makefile:1)
$(call print-location, simple_immediate, Makefile:2)
$(call print-location, compound_immediate, Makefile:6)
$(call print-location, simple_deferred, Makefile:9)
$(call print-location, compound_deferred_1, Makefile:10)
$(call print-location, compound_deferred_2, Makefile:11)
$(call print-location, overwritten, Makefile:13)
$(call print-location, appended, Makefile:15)
$(call print-location, eval_macro, Makefile:16)
$(call print-location, evaled, Makefile:17)
$(call print-location, multiple_1 multiple_2, Makefile:18 Makefile:19)

