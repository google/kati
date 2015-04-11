define comment
# PASS
endef

a:=$(comment)

foo:
	$(comment)
	echo $(a)
