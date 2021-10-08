$(shell exit 5)

define rule-template
NAMEWORKAROUND := .SHELLSTATUS
testTargetWithShellCommand:
	@echo $(shell exit 7)
	@echo $($(NAMEWORKAROUND))
	@echo "Rule ran!"
endef

$(eval $(rule-template))

$(warning $(.SHELLSTATUS))

test: testTargetWithShellCommand
	@# Suppress the "Nothing to be done for "test"." message
	@:
