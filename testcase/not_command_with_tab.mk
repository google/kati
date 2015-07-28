test: foo bar

empty:=
$(empty)
	export A=PASS_A\
with_space

foo:
	echo $$A

rule:=bar:
$(rule)
	export B=PASS_B; echo $${B}\
without_space
