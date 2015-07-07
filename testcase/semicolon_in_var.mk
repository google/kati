test: foo bar baz bazz
A:=foo: ; echo PASS
$(A)

B:=bar: ; echo PA
$(B)\
SS

baz: ; echo PA\
SS

SEMI=;
bazz: $(SEMI) echo PA\
SS
