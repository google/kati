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
# TODO: Fix
#	echo $(value $(FOOREF))
#	echo $(value @)

# TODO(go): Fix
# $(info $(value X))
