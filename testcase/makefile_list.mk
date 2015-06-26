test1:
	echo $(MAKEFILE_LIST)
	touch foo.mk

test2:
	echo $(MAKEFILE_LIST)
	touch bar.mk

test3:
	echo $(MAKEFILE_LIST)

test4: MAKEFILE_LIST=PASS
test4:
	echo $(MAKEFILE_LIST)

-include foo.mk bar.mk
-include bar.mk
-include foo.mk
-include ./././foo.mk
