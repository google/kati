GOSRC = $(wildcard *.go)

all: kati

kati: $(GOSRC)
	env $(shell go env) go build -o $@ .

test:
	ruby runtest.rb

.PHONY: test
