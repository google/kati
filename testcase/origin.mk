FOO = foo
FOO_SPACE_BAR:=foo bar
FOO_COMMA_BAR:=foo,bar
$(FOO_SPACE_BAR):=foo
$(FOO_COMMA_BAR):=foo

test:
	echo $(origin FOO)
	echo $(origin FOO BAR)
	echo $(origin FOO,BAR)
	echo $(origin UNDEFINED)
	echo $(origin PATH)

# TODO: support default, environment override, command line, and
# override.
# TODO: Also add more tests especially for += and ?=

