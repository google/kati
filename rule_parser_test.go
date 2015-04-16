package main

import (
	"reflect"
	"testing"
)

func TestRuleParser(t *testing.T) {
	for _, tc := range []struct {
		in     string
		want   Rule
		assign *AssignAST
		err    string
	}{
		{
			in: "foo: bar",
			want: Rule{
				outputs: []string{"foo"},
				inputs:  []string{"bar"},
			},
		},
		{
			in: "foo: bar baz",
			want: Rule{
				outputs: []string{"foo"},
				inputs:  []string{"bar", "baz"},
			},
		},
		{
			in: "foo:: bar",
			want: Rule{
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
			want: Rule{
				outputPatterns: []string{"%.o"},
				inputs:         []string{"%.c"},
			},
		},
		{
			in:  "foo %.o: %.c",
			err: "*** mixed implicit and normal rules: deprecated syntax",
		},
		{
			in: "foo.o: %.o: %.c %.h",
			want: Rule{
				outputs:        []string{"foo.o"},
				outputPatterns: []string{"%.o"},
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
			want: Rule{
				outputs:         []string{"foo"},
				inputs:          []string{"bar"},
				orderOnlyInputs: []string{"baz"},
			},
		},
		{
			in: "foo: CFLAGS = -g",
			want: Rule{
				outputs: []string{"foo"},
			},
			assign: &AssignAST{
				lhs: "CFLAGS",
				rhs: "-g",
				op:  "=",
			},
		},
		{
			in: "foo: CFLAGS=-g",
			want: Rule{
				outputs: []string{"foo"},
			},
			assign: &AssignAST{
				lhs: "CFLAGS",
				rhs: "-g",
				op:  "=",
			},
		},
		{
			in: "foo: CFLAGS := -g",
			want: Rule{
				outputs: []string{"foo"},
			},
			assign: &AssignAST{
				lhs: "CFLAGS",
				rhs: "-g",
				op:  ":=",
			},
		},
		{
			in: "%.o: CFLAGS := -g",
			want: Rule{
				outputPatterns: []string{"%.o"},
			},
			assign: &AssignAST{
				lhs: "CFLAGS",
				rhs: "-g",
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
		got := &Rule{}
		assign, err := got.parse(tc.in)
		if tc.err != "" {
			if err == nil {
				t.Errorf(`r.parse(%q)=_, <nil>, want _, %q`, tc.in, tc.err)
				continue
			}
			if got, want := err.Error(), tc.err; got != want {
				t.Errorf(`r.parse(%q)=_, %s, want %s`, tc.in, got, want)
			}
			continue
		}
		if err != nil {
			t.Errorf(`r.parse(%q)=_, %v; want nil error`, tc.in, err)
			continue
		}
		if !reflect.DeepEqual(*got, tc.want) {
			t.Errorf(`r.parse(%q); r=%q, want %q`, tc.in, *got, tc.want)
		}
		if tc.assign != nil {
			if assign == nil {
				t.Errorf(`r.parse(%q)=<nil>; want=%#v`, tc.in, tc.assign)
				continue
			}
			if got, want := assign, tc.assign; !reflect.DeepEqual(got, want) {
				t.Errorf(`r.parse(%q)=%#v; want=%#v`, tc.in, got, want)
			}
			continue
		}
		if assign != nil {
			t.Errorf(`r.parse(%q)=%v; want=<nil>`, tc.in, assign)
		}
	}
}
