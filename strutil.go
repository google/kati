package main

import (
	"regexp"
	"strings"
)

var isStrutilInitialized bool
var spacesRe *regexp.Regexp

func maybeInitStrutil() {
	if isStrutilInitialized {
		return
	}

	var err error
	spacesRe, err = regexp.Compile(`\s+`)
	if err != nil {
		panic(err)
	}
	isStrutilInitialized = true
}

func splitSpaces(s string) []string {
	maybeInitStrutil()
	s = strings.TrimSpace(s)
	if s == "" {
		return []string{}
	}
	return spacesRe.Split(s, -1)
}
