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
