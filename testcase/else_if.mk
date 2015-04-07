VAR:=FAIL

ifndef UNDEF
else ifndef VAR
else ifndef VAR
else
endif

ifdef UNDEF
else ifndef VAR
else ifndef VAR
else ifndef VAR
else
VAR:=PASS
endif

test:
	echo $(VAR)