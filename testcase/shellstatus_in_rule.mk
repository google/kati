$(shell exit 5)

ifdef KATI
NAMEWORKAROUND := .SHELLSTATUS
testTargetWithShellCommand:
	@echo $(shell exit 7)
	@echo $($(NAMEWORKAROUND))
else
testTargetWithShellCommand:
	@echo "Makefile:7: Kati does not support using .SHELLSTATUS inside of a rule"
endif

test: testTargetWithShellCommand
	@# Suppress the "Nothing to be done for "test"." message
	@:
