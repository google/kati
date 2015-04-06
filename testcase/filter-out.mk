objects=main1.o foo.o main2.o bar.o
mains=main1.o main2.o

# expect a list which contains all the object files not in `mains'
test:
	echo $(filter-out $(mains),$(objects))
