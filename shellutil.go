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
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

var shBuiltins = []struct {
	name    string
	pattern expr
	compact func(*funcShell, []Value) Value
}{
	{
		name: "android:rot13",
		// in repo/android/build/core/definisions.mk
		// echo $(1) | tr 'a-zA-Z' 'n-za-mN-ZA-M'
		pattern: expr{
			literal("echo "),
			matchVarref{},
			literal(" | tr 'a-zA-Z' 'n-za-mN-ZA-M'"),
		},
		compact: func(sh *funcShell, matches []Value) Value {
			return &funcShellAndroidRot13{
				funcShell: sh,
				v:         matches[0],
			}
		},
	},
	{
		name: "shell-date",
		pattern: expr{
			mustLiteralRE(`date \+(\S+)`),
		},
		compact: compactShellDate,
	},
	{
		name: "shell-date-quoted",
		pattern: expr{
			mustLiteralRE(`date "\+([^"]+)"`),
		},
		compact: compactShellDate,
	},
}

type funcShellAndroidRot13 struct {
	*funcShell
	v Value
}

func rot13(buf []byte) {
	for i, b := range buf {
		// tr 'a-zA-Z' 'n-za-mN-ZA-M'
		if b >= 'a' && b <= 'z' {
			b += 'n' - 'a'
			if b > 'z' {
				b -= 'z' - 'a' + 1
			}
		} else if b >= 'A' && b <= 'Z' {
			b += 'N' - 'A'
			if b > 'Z' {
				b -= 'Z' - 'A' + 1
			}
		}
		buf[i] = b
	}
}

func (f *funcShellAndroidRot13) Eval(w evalWriter, ev *Evaluator) error {
	abuf := newEbuf()
	fargs, err := ev.args(abuf, f.v)
	if err != nil {
		return err
	}
	rot13(fargs[0])
	w.Write(fargs[0])
	abuf.release()
	return nil
}

var (
	// ShellDateTimestamp is an timestamp used for $(shell date).
	ShellDateTimestamp time.Time
	shellDateFormatRef = map[string]string{
		"%Y": "2006",
		"%m": "01",
		"%d": "02",
		"%H": "15",
		"%M": "04",
		"%S": "05",
		"%b": "Jan",
		"%k": "15", // XXX
	}
)

type funcShellDate struct {
	*funcShell
	format string
}

func compactShellDate(sh *funcShell, v []Value) Value {
	if ShellDateTimestamp.IsZero() {
		return sh
	}
	tf, ok := v[0].(literal)
	if !ok {
		return sh
	}
	tfstr := string(tf)
	for k, v := range shellDateFormatRef {
		tfstr = strings.Replace(tfstr, k, v, -1)
	}
	return &funcShellDate{
		funcShell: sh,
		format:    tfstr,
	}
}

func (f *funcShellDate) Eval(w evalWriter, ev *Evaluator) error {
	fmt.Fprint(w, ShellDateTimestamp.Format(f.format))
	return nil
}

type buildinCommand interface {
	run(w evalWriter)
}

var errFindEmulatorDisabled = errors.New("builtin: find emulator disabled")

func parseBuiltinCommand(cmd string) (buildinCommand, error) {
	if !UseFindEmulator {
		return nil, errFindEmulatorDisabled
	}
	if strings.HasPrefix(trimLeftSpace(cmd), "build/tools/findleaves") {
		return parseFindleavesCommand(cmd)
	}
	return parseFindCommand(cmd)
}

type shellParser struct {
	cmd        string
	ungetToken string
}

func (p *shellParser) token() (string, error) {
	if p.ungetToken != "" {
		tok := p.ungetToken
		p.ungetToken = ""
		return tok, nil
	}
	p.cmd = trimLeftSpace(p.cmd)
	if len(p.cmd) == 0 {
		return "", io.EOF
	}
	if p.cmd[0] == ';' {
		tok := p.cmd[0:1]
		p.cmd = p.cmd[1:]
		return tok, nil
	}
	if p.cmd[0] == '&' {
		if len(p.cmd) == 1 || p.cmd[1] != '&' {
			return "", errFindBackground
		}
		tok := p.cmd[0:2]
		p.cmd = p.cmd[2:]
		return tok, nil
	}
	// TODO(ukai): redirect token.
	i := 0
	for i < len(p.cmd) {
		if isWhitespace(rune(p.cmd[i])) || p.cmd[i] == ';' || p.cmd[i] == '&' {
			break
		}
		i++
	}
	tok := p.cmd[0:i]
	p.cmd = p.cmd[i:]
	c := tok[0]
	if c == '\'' || c == '"' {
		if len(tok) < 2 || tok[len(tok)-1] != c {
			return "", errFindUnbalancedQuote
		}
		// todo: unquote?
		tok = tok[1 : len(tok)-1]
	}
	return tok, nil
}

func (p *shellParser) unget(s string) {
	if s != "" {
		p.ungetToken = s
	}
}

func (p *shellParser) expect(toks ...string) error {
	tok, err := p.token()
	if err != nil {
		return err
	}
	for _, t := range toks {
		if tok == t {
			return nil
		}
	}
	return fmt.Errorf("shell: token=%q; want=%q", tok, toks)
}

func (p *shellParser) expectSeq(toks ...string) error {
	for _, tok := range toks {
		err := p.expect(tok)
		if err != nil {
			return err
		}
	}
	return nil
}
