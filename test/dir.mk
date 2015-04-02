
test: foo
	echo $(dir foo)
	echo $(dir foo,bar)
	echo $(dir .)
	echo $(dir )

foo:
	mkdir foo bar
