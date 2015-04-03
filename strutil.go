package main

import (
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var strinit sync.Once
var spacesRe *regexp.Regexp

func initStrutil() {
	var err error
	spacesRe, err = regexp.Compile(`\s+`)
	if err != nil {
		panic(err)
	}
}

func splitSpaces(s string) []string {
	strinit.Do(initStrutil)
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return spacesRe.Split(s, -1)
}

func matchPattern(pat string, str string) bool {
	s := strings.SplitN(pat, "%", 2)
	if len(s) != 2 {
		return pat == str
	}
	return strings.HasPrefix(str, s[0]) && strings.HasSuffix(str, s[1])
}

func substPattern(pat string, repl string, str string) string {
	ps := strings.SplitN(pat, "%", 2)
	if len(ps) != 2 {
		if str == pat {
			return repl
		}
		return str
	}
	in := str
	trimed := str
	if ps[0] != "" {
		trimed = strings.TrimPrefix(in, ps[0])
		if trimed == in {
			return str
		}
	}
	in = trimed
	if ps[1] != "" {
		trimed = strings.TrimSuffix(in, ps[1])
		if trimed == in {
			return str
		}
	}

	rs := strings.SplitN(repl, "%", 2)
	if len(rs) != 2 {
		return repl
	}
	return rs[0] + trimed + rs[1]
}

func stripExt(s string) string {
	suf := filepath.Ext(s)
	return s[:len(s)-len(suf)]
}
