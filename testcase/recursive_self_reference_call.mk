define element_mask
 $(if $(1),
   $(if $(filter $(firstword $(1)),$(2)), T, F)
   $(call element_mask,$(wordlist 2,$(words $(1)),$(1)),$(2)))
endef

$(call element_mask,A B C,C)

