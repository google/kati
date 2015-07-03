FOO = $PATH

FOO_SPACE_BAR:=foo bar
FOO_COMMA_BAR:=foo,bar
$(FOO_SPACE_BAR):=$PATH
$(FOO_COMMA_BAR):=$PATH

FOOREF := FOO

X=$(X)

test:
	echo $(FOO)
	echo $(value FOO)
	echo $(value FOO BAR)
	echo $(value FOO,BAR)
	echo $(value $(FOOREF))
# TODO(c): Fix.
ifneq ($(KATI_VARIANT),c)
	echo $(value @)
	echo '$(value @D)'
	echo '$(value @F)'
endif

$(info $(value X))
