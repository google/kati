VAR=var
VARREF=VAR
EMPTY=
UNDEFREF=UNDEFINED

RESULT=

ifdef VAR
RESULT += PASS
endif

ifdef VAR
RESULT += PASS
else
RESULT += FAIL
endif
ifdef $(VARREF)
RESULT += PASS
else
RESULT += FAIL
endif
ifdef UNDEFINED
RESULT += FAIL
else
RESULT += PASS
endif
ifdef $(UNDEFREF)
RESULT += FAIL
else
RESULT += PASS
endif
ifdef EMPTY
RESULT += FAIL
else
RESULT += PASS
endif

ifndef VAR
RESULT += FAIL
else
RESULT += PASS
endif
ifndef $(VARREF)
RESULT += FAIL
else
RESULT += PASS
endif
ifndef UNDEFINED
RESULT += PASS
else
RESULT += FAIL
endif
ifndef $(UNDEFREF)
RESULT += PASS
else
RESULT += FAIL
endif

ifeq ($(VAR),var)
RESULT += PASS
else
RESULT += FAIL
endif
ifneq ($(VAR),var)
RESULT += FAIL
else
RESULT += PASS
endif

ifeq ($(UNDEFINED),)
RESULT += PASS
else
RESULT += FAIL
endif
ifeq (,$(UNDEFINED))
RESULT += PASS
else
RESULT += FAIL
endif

ifeq ($(VAR), var)
RESULT += PASS
else
RESULT += FAIL
endif

test:
	echo $(RESULT)
