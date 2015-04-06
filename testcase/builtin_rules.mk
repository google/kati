CFLAGS:=-g
CXXFLAGS:=-O
TARGET_ARCH:=-O2
CPPFLAGS:=-S

test1:
	touch foo.c bar.cc

test2: foo.o bar.o

# TODO: Add more builtin rules.
