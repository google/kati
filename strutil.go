// Copyright 2015 Google Inc. All rights reserved
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kati

import (
	"bytes"
	"path/filepath"
	"strings"
)

var wsbytes = [256]bool{' ': true, '\t': true, '\n': true, '\r': true}

// TODO(ukai): use unicode.IsSpace?
func isWhitespace(ch rune) bool {
	if int(ch) >= len(wsbytes) {
		return false
	}
	return wsbytes[ch]
}

func splitSpaces(s string) []string {
	var r []string
	tokStart := -1
	for i, ch := range s {
		if isWhitespace(ch) {
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
	logf("splitSpace(%q)=%q", s, r)
	return r
}

func splitSpacesBytes(s []byte) (r [][]byte) {
	tokStart := -1
	for i, ch := range s {
		if isWhitespace(rune(ch)) {
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
	logf("splitSpace(%q)=%q", s, r)
	return r
}

// TODO(ukai): use bufio.Scanner?
type wordScanner struct {
	in []byte
	s  int // word starts
	i  int // current pos
}

func newWordScanner(in []byte) *wordScanner {
	return &wordScanner{
		in: in,
	}
}

func (ws *wordScanner) Scan() bool {
	for ws.s = ws.i; ws.s < len(ws.in); ws.s++ {
		if !wsbytes[ws.in[ws.s]] {
			break
		}
	}
	if ws.s == len(ws.in) {
		return false
	}
	for ws.i = ws.s; ws.i < len(ws.in); ws.i++ {
		if wsbytes[ws.in[ws.i]] {
			break
		}
	}
	return true
}

func (ws *wordScanner) Bytes() []byte {
	return ws.in[ws.s:ws.i]
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

func substPatternBytes(pat, repl, str []byte) (pre, subst, post []byte) {
	i := bytes.IndexByte(pat, '%')
	if i < 0 {
		if bytes.Equal(str, pat) {
			return repl, nil, nil
		}
		return str, nil, nil
	}
	in := str
	trimed := str
	if i > 0 {
		trimed = bytes.TrimPrefix(in, pat[:i])
		if bytes.Equal(trimed, in) {
			return str, nil, nil
		}
	}
	in = trimed
	if i < len(pat)-1 {
		trimed = bytes.TrimSuffix(in, pat[i+1:])
		if bytes.Equal(trimed, in) {
			return str, nil, nil
		}
	}

	i = bytes.IndexByte(repl, '%')
	if i < 0 {
		return repl, nil, nil
	}

	return repl[:i], trimed, repl[i+1:]
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
		if !isWhitespace(ch) {
			return s[i:]
		}
	}
	return ""
}

func trimLeftSpaceBytes(s []byte) []byte {
	for i, ch := range s {
		if !isWhitespace(rune(ch)) {
			return s[i:]
		}
	}
	return nil
}

func trimRightSpaceBytes(s []byte) []byte {
	for i := len(s) - 1; i >= 0; i-- {
		ch := s[i]
		if !isWhitespace(rune(ch)) {
			return s[:i+1]
		}
	}
	return nil
}

func trimSpaceBytes(s []byte) []byte {
	s = trimLeftSpaceBytes(s)
	return trimRightSpaceBytes(s)
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

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
