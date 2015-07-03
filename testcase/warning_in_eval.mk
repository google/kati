warn=$(warning foo)

$(eval $(warn))
$(eval $$(warn))
$(warning bar)

test:
	echo done
