ifdef KATI
# Test that you can't bypass the readonlyness of .SHELLSTATUS
NAMEWORKAROUND := .SHELLSTATUS
$(NAMEWORKAROUND) := 5
$(info $(.SHELLSTATUS))
else
$(info Makefile:4: *** cannot assign to readonly variable: .SHELLSTATUS)
endif

test:
	@# Suppress the "Nothing to be done for "test"." message
	@:
