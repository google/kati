X:=foo \
	 bar

Y:=foo \
   \
	 bar

$(info foo \
	 bar)

test:
	echo PASS $(X) $(Y)
