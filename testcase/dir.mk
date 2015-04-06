
test: foo
	echo $(dir foo)
	echo $(dir foo,bar)
	echo $(dir .)
	echo $(dir )
	echo $(dir src/foo.c hacks)
	echo $(dir hacks src/foo.c)

foo:
	mkdir foo bar
