# TODO(ninja): Fix - ninja emits $ as '$'

foo=FAIL

$$:
	echo "$@"
