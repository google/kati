VAR=var
VARREF=VAR
EMPTY=
UNDEFREF=UNDEFINED

RESULT=

ifeq "$(VAR)" "var"
RESULT += PASS
else
RESULT += FAIL
endif
ifneq 	 "$(VAR)"  "var" 
RESULT += FAIL
else
RESULT += PASS
endif

ifeq '$(VAR)' "var"
RESULT += PASS
else
RESULT += FAIL
endif
ifeq "$(VAR)" 'var'
RESULT += PASS
else
RESULT += FAIL
endif

ifeq "$(UNDEFINED)" ""
RESULT += PASS
else
RESULT += FAIL
endif
ifeq "" "$(UNDEFINED)"
RESULT += PASS
else
RESULT += FAIL
endif

ifeq "var var" "$(VAR) $(VAR)"
RESULT += PASS
else
RESULT += FAIL
endif

test:
	echo $(RESULT)
