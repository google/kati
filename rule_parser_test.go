package main

import (
	"reflect"
	"testing"
)

func TestRuleParser(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Rule
		err  string
	} {
		{
			in:   "foo: bar",
			want: Rule{
				outputs: []string{"foo"},
				inputs:  []string{"bar"},
			},
		},
		{
			in:   "foo: bar baz",
			want: Rule{
				outputs: []string{"foo"},
				inputs:  []string{"bar", "baz"},
			},
		},
		{
			in:   "foo:: bar",
			want: Rule{
				outputs: []string{"foo"},
				inputs:  []string{"bar"},
				isDoubleColon: true,
			},
		},
		{
			in:  "foo",
			err: "*** missing separator.",
		},
		{
			in:  "%.o: %.c",
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
			in:  "foo.o: %.o: %.c %.h",
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
			in:  "foo: bar | baz",
			want: Rule{
				outputs:         []string{"foo"},
				inputs:          []string{"bar"},
				orderOnlyInputs: []string{"baz"},
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
		err := got.parse(tc.in)
		if err != tc.err {
			t.Errorf(`r.parse(%q)=%s, want %s`, tc.in, err, tc.err)
		}
		if err == "" && !reflect.DeepEqual(*got, tc.want) {
			t.Errorf(`r.parse(%q); r=%q, want %q`, tc.in, *got, tc.want)
		}
	}
}
