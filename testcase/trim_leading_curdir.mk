all: foo.bar

./foo.bar: ./%.bar: ./%.baz
	cp $< $@

./foo.baz:
	touch $@
