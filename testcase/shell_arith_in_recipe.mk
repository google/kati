test:
	echo $(filter 0,$(shell echo $$((1-1))))
