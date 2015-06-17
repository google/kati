define  define_with_space 
PASS1
endef
define  define_with_comment # foo
PASS2
endef
define  endef_with_comment
PASS3
endef # boo
define  endef_with_not_comment
PASS4
endef bar
define  endef_with_not_comment2
PASS5
endef	baz
define  endef_with_not_endef
endefPASS
endef
define  with_immediate_comment#comment
PASS6
endef
# Note: for some reason, the following is an error.
#endef#comment

test:
	echo $(define_with_space)
	echo $(define_with_comment)
	echo $(endef_with_comment)
	echo $(endef_with_not_comment)
	echo $(endef_with_not_comment2)
	echo $(endef_with_not_endef)
	echo $(with_immediate_comment)
