empty=$(info FAIL)
rec=$(empty)

ifdef rec
$(info PASS)
else
$(info FAIL)
endif
