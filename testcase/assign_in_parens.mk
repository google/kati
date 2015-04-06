$(foreach varname,x,$(eval $(varname)=PASS))
test:
	echo $(x)
