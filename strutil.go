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

func substPattern(pat string, repl string, str string) string {
	patPercentIndex := strings.IndexByte(pat, '%')
	if patPercentIndex < 0 {
		if str == pat {
			return repl
		} else {
			return str
		}
	}

	patPrefix := pat[:patPercentIndex]
	patSuffix := pat[patPercentIndex+1:]
	replPercentIndex := strings.IndexByte(repl, '%')
	if strings.HasPrefix(str, patPrefix) && strings.HasSuffix(str, patSuffix) {
		if replPercentIndex < 0 {
			return repl
		} else {
			return repl[:replPercentIndex] + str[patPercentIndex:len(str)-len(patSuffix)] + repl[replPercentIndex+1:]
		}
	}
	return str
}
