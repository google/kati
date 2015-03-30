GOSRC = $(wildcard *.go)

all: kati

kati: $(GOSRC)
	env $(shell go env) go build -o $@ .

test: all
	ruby runtest.rb

clean:
	rm -rf out kati

.PHONY: test
