# TODO(ninja): The first testcase fails due to an extra escape. We
# should be careful not to break the second case when we fix the first
# case.

test:
	echo $(shell echo \"" # "\")
	echo $$(echo \"" # "\")
