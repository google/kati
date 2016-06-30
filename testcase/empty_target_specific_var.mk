# TODO(go): https://github.com/google/kati/issues/83

test: =foo

var==foo
$(var):
	echo PASS
