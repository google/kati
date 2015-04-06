test1:
	echo TEST
ifdef UNDEFINED
	echo FAIL
else
	echo PASS
endif
	echo DONE

test2:
ifdef UNDEFINED
	echo FAIL
else
	echo PASS
endif
	echo DONE

test3:
ifndef UNDEFINED
	echo PASS
else
	echo FAIL
endif
	echo DONE
