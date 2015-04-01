# http://www.gnu.org/software/make/manual/make.html#Canned-Recipes

# canned recipes are used in gyp-generated Makefile (fixup_dep etc)

define run-echo
echo $@
endef

test:
	$(run-echo)
