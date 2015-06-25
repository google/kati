# TODO(go): Not sure how this behavior can be explained. We probably
# will not need to support bar and baz, but we probably need foo.

ECHO=@echo $@
SEMI=;
RULE=bar: ; $(EHCO)

all: foo bar baz

foo: ; $(ECHO)_1
	$(ECHO)_2

$(RULE)
	$(ECHO)_2

baz: $(SEMI) @echo $@_1
	@echo $@_2
