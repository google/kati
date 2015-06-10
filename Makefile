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

GOSRC = $(wildcard *.go)

all: kati go_test para

kati: $(GOSRC)
	env $(shell go env) go build -o $@ *.go

go_test: $(GOSRC) para
	env $(shell go env) go test *.go

para: para.cc
	$(CXX) -std=c++11 -g -O -W -Wall -MMD -o $@ $<

test: all
	ruby runtest.rb

clean:
	rm -rf out kati

.PHONY: test

-include *.d
