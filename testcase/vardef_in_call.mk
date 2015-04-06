vardef=$(eval $(1):=$(2))
$(call vardef,x,PASS)
test:
	echo $(x)
