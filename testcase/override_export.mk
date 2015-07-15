# TODO(c): it overrides "export A" and exports(?) "override B"

override export A:=override_A
export override B:=export_B

A:=make_A
B:=make_B

test:
	echo $$A
	echo $$B
	echo $(export A)
	echo $(override B)
	env | grep 'override B'
