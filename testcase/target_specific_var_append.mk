all: a b c d e f g h

a: A:=PASS_A
a: A+=A
a:
	echo A=$(A)

# Note: for some reason, make does not insert a whitespace before OK.
B:=FAIL_B
b: B+=OK
b:
	echo B=$(B)
B:=

C:=PASS_C
c: C?=FAIL_CC
c:
	echo C=$(C)

d: D?=PASS_D
d:
	echo D=$(D)

PASS_E:=PASS
e: E:=
e: E+=$(PASS_E)
e:
	echo E=$(E)
PASS_E:=FAIL

PASS_F:=FAIL
f: F=
f: F+=$(PASS_F)
f:
	echo F=$(F)
PASS_F:=PASS

PASS_G:=FAIL
G:=X
g: G+=$(PASS_G)
g:
	echo G=$(G)
PASS_G:=PASS

PASS_H:=FAIL
H=X
h: H+=$(PASS_H)
h:
	echo H=$(H)
PASS_H:=PASS
