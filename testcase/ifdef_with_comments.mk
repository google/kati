VAR:=OK

ifdef  VAR 
PASS1:=PASS1
endif # foo

ifdef  VAR # hoge
PASS2:=PASS2
endif # foo

ifeq  ($(VAR),OK) # hoge
PASS3:=PASS3
else # bar
$(error fail)
endif # foo

ifeq  ($(VAR),NOK) # hoge
$(error fail)
else # bar
PASS4:=PASS4
endif # foo

ifeq  ($(VAR),NOK)# hoge
$(error fail)
else# bar
PASS5:=PASS5
endif# foo

test:
	echo $(PASS1)
	echo $(PASS2)
	echo $(PASS3)
	echo $(PASS4)
	echo $(PASS5)
