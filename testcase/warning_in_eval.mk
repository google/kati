warn=$(warning foo)

$(eval $(warn))
# TODO: Fix
#$(eval $$(warn))
$(warning bar)
