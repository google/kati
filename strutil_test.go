package main

import (
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
	}
}
