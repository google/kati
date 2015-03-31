GOSRC = $(wildcard *.go)

all: kati go_test

kati: $(GOSRC)
	env $(shell go env) go build -o $@ .

go_test: $(GOSRC)
	env $(shell go env) go test .

test: all
	ruby runtest.rb

clean:
	rm -rf out kati

.PHONY: test
