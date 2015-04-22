PASS:=\#PASS
test1:
	echo $(PASS)

test2:
	echo \# #

define pass
\#PASS
endef
test3:
	echo $(pass)
