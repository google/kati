# TODO(go): Fix

define func
$1
endef
$(info $(call func,(PA,SS)))
$(info ${call func,{PA,SS}})
