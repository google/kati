test1:
	mkdir adir bdir
	touch adir/afile bdir/bfile afile bfile

test2: tdir/tfile tfile

tdir/tfile: adir/afile bdir/bfile
	echo $(@D)
	echo $(@F)
	echo $(<D)
	echo $(<F)
	echo $(^D)
	echo $(^F)
	echo $(+D)
	echo $(+F)
	mkdir -p tdir # for ninja.

tfile: afile bfile
	echo $(@D)
	echo $(@F)
	echo $(<D)
	echo $(<F)
	echo $(^D)
	echo $(^F)
	echo $(+D)
	echo $(+F)
