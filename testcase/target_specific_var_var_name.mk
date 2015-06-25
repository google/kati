# TODO(go): Fix. We are evaluating LHS twice.

FOO:=BAR
test: $$(FOO) := FAIL
test:
	echo $(BAR)
