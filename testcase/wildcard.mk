files = $(wildcard M*)

$(shell mkdir tmp)
files += $(wildcard tmp/../M*)
files += $(wildcard not_exist/../M*)
files += $(wildcard tmp/../M* tmp/../M*)

test:
	echo $(files)
