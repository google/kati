# TODO(c|ninja): it overrides "export A" and exports(?) "override B"
# ninja: can't export variable with space in name (by bash).

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
