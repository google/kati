
test: a/a.a b.b c/c

a/%.a:
	@echo $*

%.b:
	@echo $*

c/%:
	@echo $*
