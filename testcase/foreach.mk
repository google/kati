base := base
dirs := a b c d
dir := FAIL
files := $(foreach dir,$(dirs),$(foreach subdir,$(dirs),$(dir)/$(subdir)/$(base)))

test:
	echo $(files)
