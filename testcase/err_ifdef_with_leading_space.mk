# TODO(go): Fix

B := $(subst S, ,Sa)
ifdef $(B)
$(info PASS)
else
$(error FAIL)
endif
