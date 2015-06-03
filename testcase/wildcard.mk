files = $(wildcard M*)

$(shell mkdir tmp)
files += $(wildcard tmp/../M*)
files += $(wildcard not_exist/../M*)

test:
	echo $(files)
