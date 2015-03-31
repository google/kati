package main

import (
	"reflect"
	"testing"
)

func TestRuleParser(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Rule
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
	} {
		got := &Rule{}
		got.parse(tc.in)
		if !reflect.DeepEqual(tc.want, *got) {
			t.Errorf(`r.parse(%q)=%q, want %q`, tc.in, tc.want, *got)
		}
	}
}
