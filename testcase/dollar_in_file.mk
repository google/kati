# TODO(go-ninja): wrong shell escape.
test: $$testfile
	ls *testfile

$$testfile:
	touch \$$testfile
