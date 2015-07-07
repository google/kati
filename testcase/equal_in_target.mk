TSV:=test: A=PASS
A_EQ_B:=A=B
EQ==
$(TSV)
test: A$(EQ)B

$(A_EQ_B):
	echo $(A)
