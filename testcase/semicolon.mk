# When a line only has semicolons after variables are expanded, they
# are silently ignored, for some reason.
SEMI:=;
$(SEMI)
$(SEMI) $(SEMI)

$(foreach v,x,;)

test:
	echo PASS
