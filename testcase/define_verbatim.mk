define multiline
for i in 1 2 3 PASS; do\
 echo $$i; \
done
endef

test:
	echo "$(multiline)"
	$(multiline)

