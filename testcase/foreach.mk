dirs := a b c d
files := $(foreach dir,$(dirs),$(dir)/*)

test:
	echo $(files)
