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
	dep.cc \
	eval.cc \
	exec.cc \
	file.cc \
	file_cache.cc \
	fileutil.cc \
	func.cc \
	main.cc \
	parser.cc \
	rule.cc \
	string_piece.cc \
	string_pool.cc \
	stringprintf.cc \
	strutil.cc \
	value.cc \
	var.cc
CXX_TEST_SRCS:= \
	$(wildcard *_test.cc)
CXX_OBJS:=$(CXX_SRCS:.cc=.o)
CXX_TEST_OBJS:=$(CXX_TEST_SRCS:.cc=.o)
CXX_ALL_OBJS:=$(CXX_SRCS:.cc=.o) $(CXX_TEST_SRCS:.cc=.o)
CXX_TEST_EXES:=$(CXX_TEST_OBJS:.o=)
CXXFLAGS:=-g -W -Wall -MMD # -O

all: kati para ckati $(CXX_TEST_EXES)

kati: go_src_stamp
	GOPATH=$$(pwd)/out go install github.com/google/kati/cmd/kati
	cp out/bin/kati $@

go_src_stamp: $(GO_SRCS) cmd/*/*.go
	-rm -rf out/src/github.com/google/kati
	mkdir -p out/src/github.com/google/kati
	cp -a $(GO_SRCS) cmd out/src/github.com/google/kati
	touch $@

go_test: $(GO_SRCS) para
	go test *.go

ckati: $(CXX_OBJS)
	$(CXX) -std=c++11 $(CXXFLAGS) -o $@ $(CXX_OBJS)

$(CXX_ALL_OBJS): %.o: %.cc
	$(CXX) -c -std=c++11 $(CXXFLAGS) -o $@ $<

$(CXX_TEST_EXES): $(filter-out main.o,$(CXX_OBJS))
$(CXX_TEST_EXES): %: %.o
	$(CXX) $^ -o $@

para: para.cc
	$(CXX) -std=c++11 -g -O -W -Wall -MMD -o $@ $<

test: all go_test
	ruby runtest.rb

clean:
	rm -rf out kati ckati *.o *.d

.PHONY: test

-include *.d
