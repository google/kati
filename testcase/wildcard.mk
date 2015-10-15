# TODO(go): Fix

MAKEVER:=$(shell make --version | ruby -n0e 'puts $$_[/Make (\d)/,1]')

files = $(wildcard M*)

$(shell mkdir -p tmp)
files += $(wildcard tmp/../M*)
files += $(wildcard not_exist/../M*)
files += $(wildcard tmp/../M* not_exist/../M* tmp/../M*)
# GNU make 4 does not sort the result of $(wildcard)
ifeq ($(MAKEVER),3)
files += $(wildcard [ABC] C B A)
endif

test1:
	touch A C B

test2:
	echo $(files)
