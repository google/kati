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
	"reflect"
	"testing"
)

func TestRuleParser(t *testing.T) {
	for _, tc := range []struct {
		in     string
		tsv    *assignAST
		rhs    expr
		want   rule
		assign *assignAST
		err    string
	}{
		{
			in: "foo: bar",
			want: rule{
				outputs: []string{"foo"},
				inputs:  []string{"bar"},
			},
		},
		{
			in: "foo: bar baz",
			want: rule{
				outputs: []string{"foo"},
				inputs:  []string{"bar", "baz"},
			},
		},
		{
			in: "foo:: bar",
			want: rule{
				outputs:       []string{"foo"},
				inputs:        []string{"bar"},
				isDoubleColon: true,
			},
		},
		{
			in:  "foo",
			err: "*** missing separator.",
		},
		{
			in: "%.o: %.c",
			want: rule{
				outputs:        []string{},
				outputPatterns: []pattern{pattern{suffix: ".o"}},
				inputs:         []string{"%.c"},
			},
		},
		{
			in:  "foo %.o: %.c",
			err: "*** mixed implicit and normal rules: deprecated syntax",
		},
		{
			in: "foo.o: %.o: %.c %.h",
			want: rule{
				outputs:        []string{"foo.o"},
				outputPatterns: []pattern{pattern{suffix: ".o"}},
				inputs:         []string{"%.c", "%.h"},
			},
		},
		{
			in:  "%.x: %.y: %.z",
			err: "*** mixed implicit and normal rules: deprecated syntax",
		},
		{
			in:  "foo.o: : %.c",
			err: "*** missing target pattern.",
		},
		{
			in:  "foo.o: %.o %.o: %.c",
			err: "*** multiple target patterns.",
		},
		{
			in:  "foo.o: foo.o: %.c",
			err: "*** target pattern contains no '%'.",
		},
		{
			in: "foo: bar | baz",
			want: rule{
				outputs:         []string{"foo"},
				inputs:          []string{"bar"},
				orderOnlyInputs: []string{"baz"},
			},
		},
		{
			in:  "foo: CFLAGS =",
			rhs: expr{literal("-g")},
			want: rule{
				outputs: []string{"foo"},
			},
			assign: &assignAST{
				lhs: literal("CFLAGS"),
				rhs: literal("-g"),
				op:  "=",
			},
		},
		{
			in: "foo:",
			tsv: &assignAST{
				lhs: literal("CFLAGS"),
				rhs: literal("-g"),
				op:  "=",
			},
			want: rule{
				outputs: []string{"foo"},
			},
			assign: &assignAST{
				lhs: literal("CFLAGS"),
				rhs: literal("-g"),
				op:  "=",
			},
		},
		{
			in:  "foo: CFLAGS=",
			rhs: expr{literal("-g")},
			want: rule{
				outputs: []string{"foo"},
			},
			assign: &assignAST{
				lhs: literal("CFLAGS"),
				rhs: literal("-g"),
				op:  "=",
			},
		},
		{
			in:  "foo: CFLAGS :=",
			rhs: expr{literal("-g")},
			want: rule{
				outputs: []string{"foo"},
			},
			assign: &assignAST{
				lhs: literal("CFLAGS"),
				rhs: literal("-g"),
				op:  ":=",
			},
		},
		{
			in:  "%.o: CFLAGS :=",
			rhs: expr{literal("-g")},
			want: rule{
				outputs:        []string{},
				outputPatterns: []pattern{pattern{suffix: ".o"}},
			},
			assign: &assignAST{
				lhs: literal("CFLAGS"),
				rhs: literal("-g"),
				op:  ":=",
			},
		},
		{
			in: "%.o:",
			tsv: &assignAST{
				lhs: literal("CFLAGS"),
				rhs: literal("-g"),
				op:  ":=",
			},
			want: rule{
				outputs:        []string{},
				outputPatterns: []pattern{pattern{suffix: ".o"}},
			},
			assign: &assignAST{
				lhs: literal("CFLAGS"),
				rhs: literal("-g"),
				op:  ":=",
			},
		},
		/* TODO
		{
			in:  "foo.o: %.c: %.c",
			err: "*** target 'foo.o' doesn't match the target pattern",
		},
		*/
	} {
		got := &rule{}
		assign, err := got.parse([]byte(tc.in), tc.tsv, tc.rhs)
		if tc.err != "" {
			if err == nil {
				t.Errorf(`r.parse(%q, %v)=_, <nil>, want _, %q`, tc.in, tc.rhs, tc.err)
				continue
			}
			if got, want := err.Error(), tc.err; got != want {
				t.Errorf(`r.parse(%q, %v)=_, %s, want %s`, tc.in, tc.rhs, got, want)
			}
			continue
		}
		if err != nil {
			t.Errorf(`r.parse(%q, %v)=_, %v; want nil error`, tc.in, tc.rhs, err)
			continue
		}
		if !reflect.DeepEqual(*got, tc.want) {
			t.Errorf(`r.parse(%q, %v); r=%#v, want %#v`, tc.in, tc.rhs, *got, tc.want)
		}
		if tc.assign != nil {
			if assign == nil {
				t.Errorf(`r.parse(%q, %v)=<nil>; want=%#v`, tc.in, tc.rhs, tc.assign)
				continue
			}
			if got, want := assign, tc.assign; !reflect.DeepEqual(got, want) {
				t.Errorf(`r.parse(%q, %v)=%#v; want=%#v`, tc.in, tc.rhs, got, want)
			}
			continue
		}
		if assign != nil {
			t.Errorf(`r.parse(%q, %v)=%v; want=<nil>`, tc.in, tc.rhs, assign)
		}
	}
}
