# TODO(go|ninja): This test is only for ckati. ninja: multiple problems
# go: symlink support isn't enough.
# ninja: find . finds ninja temporary files
# ninja: escaping ! doesn't seem to be working
# ninja: stderr gets reordered

ifeq ($(shell uname),Darwin)
USE_GNU_FIND:=
else
USE_GNU_FIND:=1
endif

define run_find
@echo $$ '$(strip $(1))'
@echo $(shell $(1))
endef

test1:
	mkdir testdir
	touch testdir/file1
	touch testdir/file2
	mkdir testdir/dir1
	touch testdir/dir1/file1
	touch testdir/dir1/file2
	touch testdir/dir1/file3
	mkdir testdir/dir2
	touch testdir/dir2/file1
	touch testdir/dir2/file2
	touch testdir/dir2/file3
	ln -s ../dir1/file1 testdir/dir2/link1
	ln -s ../../testdir/dir1 testdir/dir2/link2
	ln -s broken testdir/dir2/link3
	mkdir -p build/tools
	cp ../../testcase/tools/findleaves.py build/tools

	mkdir -p testdir3/b/c/d
	ln -s b testdir3/a
	touch testdir3/b/c/d/e

	mkdir -p testdir4/a/b
	ln -s self testdir4/self
	ln -s .. testdir4/a/b/c
	ln -s b testdir4/a/l

	mkdir -p testdir5
	ln -s a testdir5/a
	ln -s b testdir5/c
	ln -s c testdir5/b

test2:
	@echo no options
	$(call run_find, find testdir)
	$(call run_find, find .)
ifeq ($(USE_GNU_FIND),1)
	$(call run_find, find ./)
	$(call run_find, find .///)
	$(call run_find, find )
	$(call run_find, find ./.)
	$(call run_find, find ././)
endif
	$(call run_find, find testdir/../testdir)
	@echo print
	$(call run_find, find testdir -print)
	@echo conditiions
	$(call run_find, find testdir -name foo)
	$(call run_find, find testdir -name file1)
	$(call run_find, find testdir -name "file1")
	$(call run_find, find testdir -name "file1")
	$(call run_find, find testdir -name "*1")
	$(call run_find, find testdir -name "*1" -and -name "file*")
	$(call run_find, find testdir -name "*1" -or -name "file*")
	$(call run_find, find testdir -name "*1" -or -type f)
	$(call run_find, find testdir -name "*1" -or -not -type f)
	$(call run_find, find testdir -name "*1" -or \! -type f)
	$(call run_find, find testdir -name "*1" -or -type d)
	$(call run_find, find testdir -name "*1" -or -type l)
	$(call run_find, find testdir -name "*1" -a -type l -o -name "dir*")
	$(call run_find, find testdir -name "dir*" -o -name "*1" -a -type l)
	$(call run_find, find testdir \( -name "dir*" -o -name "*1" \) -a -type f)
	@echo cd
	$(call run_find, cd testdir && find)
	$(call run_find, cd testdir/// && find .)
	$(call run_find, cd testdir///dir1// && find .///)
	$(call run_find, cd testdir && find ../testdir)
	@echo test
	$(call run_find, test -d testdir && find testdir)
	$(call run_find, if [ -d testdir ] ; then find testdir ; fi)
	$(call run_find, if [ -d testdir ]; then find testdir; fi)
	$(call run_find, if [ -d testdir ]; then cd testdir && find .; fi)
	$(call run_find, test -d testdir//dir1/// && find testdir///dir1///)
	$(call run_find, test -d testdir//.///dir1/// && find testdir//.///dir1///)
	@echo prune
	$(call run_find, find testdir -name dir2 -prune -o -name file1)
	@echo multi
	$(call run_find, find testdir testdir)
	@echo symlink
	$(call run_find, find -L testdir -type f)
	$(call run_find, find -L testdir -type d)
	$(call run_find, find -L testdir -type l)
	$(call run_find, cd testdir; find -L . -type f)
	$(call run_find, cd testdir; find -L . -type d)
	$(call run_find, cd testdir; find -L . -type l)
	@echo maxdepth
	$(call run_find, find testdir -maxdepth 1)
	$(call run_find, find testdir -maxdepth 2)
	$(call run_find, find testdir -maxdepth 0)
	$(call run_find, find testdir -maxdepth hoge)
	$(call run_find, find testdir -maxdepth 1hoge)
	$(call run_find, find testdir -maxdepth -1)
	@echo findleaves
	$(call run_find, build/tools/findleaves.py testdir file1)
	$(call run_find, build/tools/findleaves.py testdir file3)
	$(call run_find, build/tools/findleaves.py --prune=dir1 testdir file3)
	$(call run_find, build/tools/findleaves.py --prune=dir1 --prune=dir2 testdir file3)
	$(call run_find, build/tools/findleaves.py --mindepth=1 testdir file1)
	$(call run_find, build/tools/findleaves.py --mindepth=2 testdir file1)
	$(call run_find, build/tools/findleaves.py --mindepth=3 testdir file1)
	$(call run_find, build/tools/findleaves.py --mindepth=2 testdir file1)
	$(call run_find, build/tools/findleaves.py --prune=dir1 --dir=testdir file1)
	$(call run_find, build/tools/findleaves.py --prune=dir1 --dir=testdir file3 link3)
	@echo missing chdir / testdir
	$(call run_find, cd xxx && find .)
	$(call run_find, if [ -d xxx ]; then find .; fi)

test3:
	$(call run_find, find testdir3/a/c)
	$(call run_find, if [ -d testdir3/a/c ]; then find testdir3/a/c; fi)
	$(call run_find, cd testdir3/a/c && find .)
	$(call run_find, build/tools/findleaves.py testdir3 e)

test4:
	$(call run_find, find -L testdir4)

test5:
	$(call run_find, find -L testdir5)
	$(call run_find, build/tools/findleaves.py testdir5 x)
