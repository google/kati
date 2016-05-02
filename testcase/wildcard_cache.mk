# TODO(c): Fix this. Maybe $(wildcard) always runs at eval-phase.

# GNU make 4 agrees with ckati.
MAKEVER:=$(shell make --version | ruby -n0e 'puts $$_[/Make (\d)/,1]')
ifeq ($(MAKE)$(MAKEVER),make4)
$(error test skipped)
endif

files = $(wildcard *,*)

# if make starts without foo,bar, it will be empty, although expect foo,bar.
test: foo,bar
	echo $(files)
	echo $(wildcard foo*)

# first $(files) will be empty since no foo,bar exists.
# second $(files) expects foo, but empty.
foo,bar:
	echo $(files)
	touch foo,bar
	echo $(files)

$(shell mkdir dir)
$(info $(wildcard dir/not_exist))
$(shell touch dir/file)
# This should show nothing.
$(info $(wildcard dir/file))
