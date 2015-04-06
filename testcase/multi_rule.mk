test: foo.c

# simplified case for gyp-generated action targets
# with 'process_outputs_as_sources': 1
# and 'hard_dependency': 1
foo.c: CFLAGS:=-g
foo.c:
	echo generating foo.c

outputs := foo.c

$(outputs): |


