all: foo bar baz hoge

foo: A:=PASS_A
foo: A+=A
foo:
	echo A=$(A)

# Note: for some reason, make does not insert a whitespace before OK.
B:=FAIL_B
bar: B+=OK
bar:
	echo B=$(B)
B:=

C:=PASS_C
baz: C?=FAIL_CC
baz:
	echo C=$(C)

hoge: D?=PASS_D
hoge:
	echo D=$(D)
