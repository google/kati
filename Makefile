# Copyright 2015 Google Inc. All rights reserved
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

GO_SRCS:=$(wildcard *.go)
CXX_SRCS:= \
	ast.cc \
	command.cc \
	dep.cc \
	eval.cc \
	exec.cc \
	file.cc \
	file_cache.cc \
	fileutil.cc \
	find.cc \
	flags.cc \
	func.cc \
	log.cc \
	main.cc \
	ninja.cc \
	parser.cc \
	rule.cc \
	stats.cc \
	string_piece.cc \
	stringprintf.cc \
	strutil.cc \
	symtab.cc \
	timeutil.cc \
	value.cc \
	var.cc \
	version.cc
CXX_TEST_SRCS:= \
	$(wildcard *_test.cc)
CXX_OBJS:=$(CXX_SRCS:.cc=.o)
CXX_TEST_OBJS:=$(CXX_TEST_SRCS:.cc=.o)
CXX_ALL_OBJS:=$(CXX_SRCS:.cc=.o) $(CXX_TEST_SRCS:.cc=.o)
CXX_TEST_EXES:=$(CXX_TEST_OBJS:.o=)
CXXFLAGS:=-g -W -Wall -MMD
CXXFLAGS+=-O -DNOLOG
#CXXFLAGS+=-pg

all: kati ckati $(CXX_TEST_EXES)

kati: go_src_stamp
	GOPATH=$$(pwd)/out:$${GOPATH} go install -ldflags "-X github.com/google/kati.gitVersion $(shell git rev-parse HEAD)" github.com/google/kati/cmd/kati
	cp out/bin/kati $@

go_src_stamp: $(GO_SRCS) cmd/*/*.go
	-rm -rf out/src/github.com/google/kati
	mkdir -p out/src/github.com/google/kati
	cp -a $(GO_SRCS) cmd out/src/github.com/google/kati
	GOPATH=$$(pwd)/out:$${GOPATH} go get github.com/google/kati/cmd/kati
	touch $@

go_test: $(GO_SRCS)
	GOPATH=$$(pwd)/out:$${GOPATH} go test *.go

ckati: $(CXX_OBJS)
	$(CXX) -std=c++11 $(CXXFLAGS) -o $@ $(CXX_OBJS)

$(CXX_ALL_OBJS): %.o: %.cc
	$(CXX) -c -std=c++11 $(CXXFLAGS) -o $@ $<

$(CXX_TEST_EXES): $(filter-out main.o,$(CXX_OBJS))
$(CXX_TEST_EXES): %: %.o
	$(CXX) $^ -o $@

version.cc: .git/HEAD .git/index
	echo 'const char* kGitVersion = "$(shell git rev-parse HEAD)";' > $@

test: all go_test
	ruby runtest.rb

clean:
	rm -rf out kati ckati *.o *.d go_src_stamp

.PHONY: test

-include *.d
