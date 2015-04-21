package main

import (
	"bytes"
	"path/filepath"
	"strings"
)

func splitSpaces(s string) []string {
	var r []string
	tokStart := -1
	for i, ch := range s {
		if ch == ' ' || ch == '\t' {
			if tokStart >= 0 {
				r = append(r, s[tokStart:i])
				tokStart = -1
			}
		} else {
			if tokStart < 0 {
				tokStart = i
			}
		}
	}
	if tokStart >= 0 {
		r = append(r, s[tokStart:])
	}
	Log("splitSpace(%q)=%q", s, r)
	return r
}

func splitSpacesBytes(s []byte) (r [][]byte) {
	tokStart := -1
	for i, ch := range s {
		if ch == ' ' || ch == '\t' {
			if tokStart >= 0 {
				r = append(r, s[tokStart:i])
				tokStart = -1
			}
		} else {
			if tokStart < 0 {
				tokStart = i
			}
		}
	}
	if tokStart >= 0 {
		r = append(r, s[tokStart:])
	}
	Log("splitSpace(%q)=%q", s, r)
	return r
}

func matchPattern(pat, str string) bool {
	i := strings.IndexByte(pat, '%')
	if i < 0 {
		return pat == str
	}
	return strings.HasPrefix(str, pat[:i]) && strings.HasSuffix(str, pat[i+1:])
}

func matchPatternBytes(pat, str []byte) bool {
	i := bytes.IndexByte(pat, '%')
	if i < 0 {
		return bytes.Equal(pat, str)
	}
	return bytes.HasPrefix(str, pat[:i]) && bytes.HasSuffix(str, pat[i+1:])
}

func substPattern(pat, repl, str string) string {
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

func substPatternBytes(pat, repl, str []byte) []byte {
	ps := bytes.SplitN(pat, []byte{'%'}, 2)
	if len(ps) != 2 {
		if bytes.Equal(str, pat) {
			return repl
		}
		return str
	}
	in := str
	trimed := str
	if len(ps[0]) != 0 {
		trimed = bytes.TrimPrefix(in, ps[0])
		if bytes.Equal(trimed, in) {
			return str
		}
	}
	in = trimed
	if len(ps[1]) != 0 {
		trimed = bytes.TrimSuffix(in, ps[1])
		if bytes.Equal(trimed, in) {
			return str
		}
	}

	rs := bytes.SplitN(repl, []byte{'%'}, 2)
	if len(rs) != 2 {
		return repl
	}

	r := make([]byte, 0, len(rs[0])+len(trimed)+len(rs[1])+1)
	r = append(r, rs[0]...)
	r = append(r, trimed...)
	r = append(r, rs[1]...)
	return r
}

func substRef(pat, repl, str string) string {
	if strings.IndexByte(pat, '%') >= 0 && strings.IndexByte(repl, '%') >= 0 {
		return substPattern(pat, repl, str)
	}
	str = strings.TrimSuffix(str, pat)
	return str + repl
}

func stripExt(s string) string {
	suf := filepath.Ext(s)
	return s[:len(s)-len(suf)]
}

func trimLeftSpace(s string) string {
	for i, ch := range s {
		if ch != ' ' && ch != '\t' {
			return s[i:]
		}
	}
	return ""
}

func trimLeftSpaceBytes(s []byte) []byte {
	for i, ch := range s {
		if ch != ' ' && ch != '\t' {
			return s[i:]
		}
	}
	return nil
}

// Strip leading sequences of './' from file names, so that ./file
// and file are considered to be the same file.
// From http://www.gnu.org/software/make/manual/make.html#Features
func trimLeadingCurdir(s string) string {
	for strings.HasPrefix(s, "./") {
		s = s[2:]
	}
	return s
}
