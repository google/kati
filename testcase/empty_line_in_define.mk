define foo
echo hoge

endef

test:
	$(foo) $(foo)
