# TODO(go): Fix

A := a # comment

ifdef $(A)
$(error FAIL)
else
$(info PASS)
endif

a := b
ifdef $(A)
$(info PASS)
else
$(error FAIL)
endif

ifdef a # comment
$(info PASS)
else
$(error FAIL)
endif
