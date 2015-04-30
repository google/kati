GOSRC = $(wildcard *.go)

all: kati go_test para

kati: $(GOSRC)
	env $(shell go env) go build -o $@ *.go

go_test: $(GOSRC)
	env $(shell go env) go test *.go

para: para.cc
	$(CXX) -std=c++11 -g -O -MMD -o $@ $<

test: all
	ruby runtest.rb

clean:
	rm -rf out kati

.PHONY: test

-include *.d
