define or1
$(or , \
 ,,)
endef

define or2
$(or ,,, \
)
endef

define or3
$(or  ,  ,  ,)
endef

test:
	echo $(if $(call or1),FAIL,PASS)_or1
	echo $(if $(call or2),FAIL,PASS)_or2
	echo $(if $(call or3),FAIL,PASS)_or3
