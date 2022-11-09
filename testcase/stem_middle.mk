
test: a/a.a b.b c/c

# ninja always makes the folders leading up to the outputs,
# so add mkdirs to match that functionality in make

a/%.a:
	@mkdir -p a
	@echo $*

%.b:
	@echo $*

c/%:
	@mkdir -p c
	@echo $*
