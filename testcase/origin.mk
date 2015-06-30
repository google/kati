FOO = foo
FOO_SPACE_BAR:=foo bar
FOO_COMMA_BAR:=foo,bar
$(FOO_SPACE_BAR):=foo
$(FOO_COMMA_BAR):=foo
FOOREF := FOO

test:
	echo $(origin FOO)
	echo $(origin FOO BAR)
	echo $(origin FOO,BAR)
	echo $(origin UNDEFINED)
	echo $(origin PATH)
	echo $(origin MAKEFILE_LIST)
	echo $(origin CC)
	echo $(origin $(FOOREF))

# TODO: support environment override, command line, and override.
# TODO: Also add more tests especially for += and ?=

