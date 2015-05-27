package main

import "testing"

func TestStripShellComment(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{
			in:   `foo`,
			want: `foo`,
		},
		{
			in:   `foo # bar`,
			want: `foo `,
		},
		{
			in:   `foo '# bar'`,
			want: `foo '# bar'`,
		},
		{
			in:   `foo '\'# bar'`,
			want: `foo '\'`,
		},
		{
			in:   `foo "# bar"`,
			want: `foo "# bar"`,
		},
		{
			in:   `foo "\"# bar"`,
			want: `foo "\"# bar"`,
		},
		{
			in:   `foo "\\"# bar"`,
			want: `foo "\\"`,
		},
		{
			in:   "foo `# bar`",
			want: "foo `# bar`",
		},
		{
			in:   "foo `\\`# bar`",
			want: "foo `\\`# bar`",
		},
		{
			in:   "foo `\\\\`# bar`",
			want: "foo `\\\\`",
		},
	} {
		got := stripShellComment(tc.in)
		if got != tc.want {
			t.Errorf(`stripShellComment(%q)=%q, want %q`, tc.in, got, tc.want)
		}
	}
}
