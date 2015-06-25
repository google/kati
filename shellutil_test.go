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
	"testing"
	"time"
)

func TestRot13(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{
			in:   "PRODUCT_PACKAGE_OVERLAYS",
			want: "CEBQHPG_CNPXNTR_BIREYNLF",
		},
		{
			in:   "product_name",
			want: "cebqhpg_anzr",
		},
	} {
		buf := []byte(tc.in)
		rot13(buf)
		if got, want := string(buf), tc.want; got != want {
			t.Errorf("rot13(%q) got=%q; want=%q", tc.in, got, want)
		}
	}
}

func TestShellDate(t *testing.T) {
	ts := ShellDateTimestamp
	ShellDateTimestamp = time.Now()
	defer func() {
		ShellDateTimestamp = ts
	}()
	for _, tc := range []struct {
		sharg  literal
		format string
	}{
		{
			sharg:  literal("date +%Y-%m-%d"),
			format: "2006-01-02",
		},
		{
			sharg:  literal("date +%Y%m%d.%H%M%S"),
			format: "20060102.150405",
		},
		{
			sharg:  literal(`date "+%d %b %Y %k:%M"`),
			format: "02 Jan 2006 15:04",
		},
	} {
		var matched bool
		for _, b := range shBuiltins {
			if b.name != "shell-date" && b.name != "shell-date-quoted" {
				continue
			}
			m, ok := matchExpr(expr{tc.sharg}, b.pattern)
			if !ok {
				t.Logf("%s not match with %s", b.name, tc.sharg)
				continue
			}
			f := &funcShell{
				fclosure: fclosure{
					args: []Value{
						literal("(shell"),
						tc.sharg,
					},
				},
			}
			v := b.compact(f, m)
			sd, ok := v.(*funcShellDate)
			if !ok {
				t.Errorf("%s: matched %s but not compacted", tc.sharg, b.name)
				continue
			}
			if got, want := sd.format, tc.format; got != want {
				t.Errorf("%s: format=%q, want=%q - %s", tc.sharg, got, want, b.name)
				continue
			}
			matched = true
			break
		}
		if !matched {
			t.Errorf("%s: not matched", tc.sharg)
		}
	}
}
