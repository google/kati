base := base
dirs := a b c d
dir := FAIL
comma := ,
files := $(foreach dir,$(dirs),$(foreach subdir,$(dirs),$(dir)/$(subdir)/$(base)))
ifdef KATI
files2 := $(KATI_foreach_sep dir,$(comma) ,$(dirs),"$(dir)")
else
# Since make doesn't have the function, manually set the result.
files2 := "a", "b", "c", "d"
endif

test:
	echo $(files)
	echo $(files2)
