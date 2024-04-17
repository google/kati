$(KATI_visibility_prefix FOO, Makefile)

BAR := $(FOO)

test:
	echo '$(BAR)'
