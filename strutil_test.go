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

package main

import (
	"fmt"
	"reflect"
	"testing"
)

func TestSplitSpaces(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{
			in:   "foo",
			want: []string{"foo"},
		},
		{
			in: "  	 ",
			want: nil,
		},
		{
			in: "  foo 	  bar 	",
			want: []string{"foo", "bar"},
		},
		{
			in:   "  foo bar",
			want: []string{"foo", "bar"},
		},
		{
			in:   "foo bar  ",
			want: []string{"foo", "bar"},
		},
	} {
		got := splitSpaces(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf(`splitSpaces(%q)=%q, want %q`, tc.in, got, tc.want)
		}
	}
}

func TestWordScanner(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{
			in:   "foo",
			want: []string{"foo"},
		},
		{
			in: "  	 ",
			want: nil,
		},
		{
			in: "  foo 	  bar 	",
			want: []string{"foo", "bar"},
		},
		{
			in:   "  foo bar",
			want: []string{"foo", "bar"},
		},
		{
			in:   "foo bar  ",
			want: []string{"foo", "bar"},
		},
	} {
		ws := newWordScanner([]byte(tc.in))
		var got []string
		for ws.Scan() {
			got = append(got, string(ws.Bytes()))
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf(`wordScanner(%q)=%q, want %q`, tc.in, got, tc.want)
		}
	}
}

func TestSubstPattern(t *testing.T) {
	for _, tc := range []struct {
		pat  string
		repl string
		in   string
		want string
	}{
		{
			pat:  "%.c",
			repl: "%.o",
			in:   "x.c",
			want: "x.o",
		},
		{
			pat:  "c.%",
			repl: "o.%",
			in:   "c.x",
			want: "o.x",
		},
		{
			pat:  "%.c",
			repl: "%.o",
			in:   "x.c.c",
			want: "x.c.o",
		},
		{
			pat:  "%.c",
			repl: "%.o",
			in:   "x.x y.c",
			want: "x.x y.o",
		},
		{
			pat:  "%.%.c",
			repl: "OK",
			in:   "x.%.c",
			want: "OK",
		},
		{
			pat:  "x.c",
			repl: "XX",
			in:   "x.c",
			want: "XX",
		},
		{
			pat:  "x.c",
			repl: "XX",
			in:   "x.c.c",
			want: "x.c.c",
		},
		{
			pat:  "x.c",
			repl: "XX",
			in:   "x.x.c",
			want: "x.x.c",
		},
	} {
		got := substPattern(tc.pat, tc.repl, tc.in)
		if got != tc.want {
			t.Errorf(`substPattern(%q,%q,%q)=%q, want %q`, tc.pat, tc.repl, tc.in, got, tc.want)
		}

		got = string(substPatternBytes([]byte(tc.pat), []byte(tc.repl), []byte(tc.in)))
		if got != tc.want {
			fmt.Printf("substPatternBytes(%q,%q,%q)=%q, want %q\n", tc.pat, tc.repl, tc.in, got, tc.want)
			t.Errorf(`substPatternBytes(%q,%q,%q)=%q, want %q`, tc.pat, tc.repl, tc.in, got, tc.want)
		}
	}
}
