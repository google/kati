test: a/./b ./x

a/./b:
	echo $@
	mkdir -p a # for ninja.

././x:
	echo $@
