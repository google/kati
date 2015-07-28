define func
$(11)$(12)$(13)$(14)
endef

test:
	echo $(call func,1,2,3,4,5,6,7,8,9,10,P,A,S,S)
