# TODO(all/test2): Fix. We probably need to assume foo.y exists as there's a rule
# to generate it.

test1:
	touch foo.x

test2: foo.z

%.z: %.y
	cp $< $@

%.y: %.x
	cp $< $@
