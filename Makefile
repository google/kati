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
