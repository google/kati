package main

import (
	"reflect"
	"testing"
)

func TestParseExpr(t *testing.T) {
	for _, tc := range []struct {
		in    string
		args  []string
		rest  string
		isErr bool
	}{
		{
			in:    "foo",
			isErr: true,
		},
		{
			in:   "(foo)",
			args: []string{"foo"},
		},
		{
			in:   "{foo}",
			args: []string{"foo"},
		},
		{
			in:   "(lhs,rhs)",
			args: []string{"lhs", "rhs"},
		},
		{
			in:   "(subst $(space),$(,),$(foo))/bar",
			args: []string{"subst $(space)", "$(,)", "$(foo)"},
			rest: "/bar",
		},
	} {
		args, rest, err := parseExpr(tc.in)
		if tc.isErr {
			if err == nil {
				t.Errorf(`parseExpr(%q)=_, _, nil; want error`, tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf(`parseExpr(%q)=_, _, %v; want nil error`, tc.in, err)
			continue
		}
		if got, want := args, tc.args; !reflect.DeepEqual(got, want) {
			t.Errorf(`parseExpr(%q)=%q, _, _; want %q, _, _`, tc.in, got, want)
		}
		if got, want := tc.in[rest:], tc.rest; got != want {
			t.Errorf(`parseExpr(%q)=_, %q, _; want _, %q, _`, tc.in, got, want)
		}
	}
}
