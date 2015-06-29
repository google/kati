files = $(wildcard M*)

$(shell mkdir -p tmp)
files += $(wildcard tmp/../M*)
files += $(wildcard not_exist/../M*)
files += $(wildcard tmp/../M* not_exist/../M* tmp/../M* [ABC] C B A)

test1:
	touch A C B

test2:
	echo $(files)
