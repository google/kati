MAKEVER:=$(shell make --version | ruby -n0e 'puts $$_[/Make (\d)/,1]')

# GNU make 4 doesn't sort glob results.
ifeq ($(MAKEVER,4))

$(info test skipped)

else

test1:
	echo '$$(info foo)' > foo.d
	echo '$$(info bar)' > bar.d

test2:
	echo $(wildcard *.d)

-include *.d

endif
