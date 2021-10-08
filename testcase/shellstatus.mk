# Check the value of .SHELLSTATUS before $(shell) has run
A := $(.SHELLSTATUS)

$(shell exit 0)
B := $(.SHELLSTATUS)

$(shell exit 1)
C := $(.SHELLSTATUS)

# .SHELLSTATUS is global across makefiles
$(file >nested.mk,$$(shell exit 2))
include nested.mk
D := $(.SHELLSTATUS)

ruletest: temp := $(shell exit 3)
ruletest: E := $(.SHELLSTATUS)
ruletest:
	@echo $(E)

$(shell exit 0)

test: ruletest
	echo $(A) $(B) $(C) $(D) $(flavor .SHELLSTATUS)
