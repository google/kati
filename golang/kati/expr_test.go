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

func TestParseExpr(t *testing.T) {
	for _, tc := range []struct {
		in    string
		val   Value
		isErr bool
	}{
		{
			in:  "foo",
			val: literal("foo"),
		},
		{
			in:  "(foo)",
			val: literal("(foo)"),
		},
		{
			in:  "{foo}",
			val: literal("{foo}"),
		},
		{
			in:  "$$",
			val: literal("$"),
		},
		{
			in:  "foo$$bar",
			val: literal("foo$bar"),
		},
		{
			in:  "$foo",
			val: expr{&varref{varname: literal("f")}, literal("oo")},
		},
		{
			in:  "$(foo)",
			val: &varref{varname: literal("foo"), paren: '('},
		},
		{
			in: "$(foo:.c=.o)",
			val: varsubst{
				varname: literal("foo"),
				pat:     literal(".c"),
				subst:   literal(".o"),
				paren:   '(',
			},
		},
		{
			in: "$(subst $(space),$(,),$(foo))/bar",
			val: expr{
				&funcSubst{
					fclosure: fclosure{
						args: []Value{
							literal("(subst"),
							&varref{
								varname: literal("space"),
								paren:   '(',
							},
							&varref{
								varname: literal(","),
								paren:   '(',
							},
							&varref{
								varname: literal("foo"),
								paren:   '(',
							},
						},
					},
				},
				literal("/bar"),
			},
		},
		{
			in: "$(subst $(space),$,,$(foo))",
			val: &funcSubst{
				fclosure: fclosure{
					args: []Value{
						literal("(subst"),
						&varref{
							varname: literal("space"),
							paren:   '(',
						},
						&varref{
							varname: literal(""),
						},
						expr{
							literal(","),
							&varref{
								varname: literal("foo"),
								paren:   '(',
							},
						},
					},
				},
			},
		},
		{
			in: `$(shell echo '()')`,
			val: &funcShell{
				fclosure: fclosure{
					args: []Value{
						literal("(shell"),
						literal("echo '()'"),
					},
				},
			},
		},
		{
			in: `${shell echo '()'}`,
			val: &funcShell{
				fclosure: fclosure{
					args: []Value{
						literal("{shell"),
						literal("echo '()'"),
					},
				},
			},
		},
		{
			in: `$(shell echo ')')`,
			val: expr{
				&funcShell{
					fclosure: fclosure{
						args: []Value{
							literal("(shell"),
							literal("echo '"),
						},
					},
				},
				literal("')"),
			},
		},
		{
			in: `${shell echo ')'}`,
			val: &funcShell{
				fclosure: fclosure{
					args: []Value{
						literal("{shell"),
						literal("echo ')'"),
					},
				},
			},
		},
		{
			in: `${shell echo '}'}`,
			val: expr{
				&funcShell{
					fclosure: fclosure{
						args: []Value{
							literal("{shell"),
							literal("echo '"),
						},
					},
				},
				literal("'}"),
			},
		},
		{
			in: `$(shell make --version | ruby -n0e 'puts $$_[/Make (\d)/,1]')`,
			val: &funcShell{
				fclosure: fclosure{
					args: []Value{
						literal("(shell"),
						literal(`make --version | ruby -n0e 'puts $_[/Make (\d)/,1]'`),
					},
				},
			},
		},
		{
			in: `$(and ${TRUE}, $(X)   )`,
			val: &funcAnd{
				fclosure: fclosure{
					args: []Value{
						literal("(and"),
						&varref{
							varname: literal("TRUE"),
							paren:   '{',
						},
						&varref{
							varname: literal("X"),
							paren:   '(',
						},
					},
				},
			},
		},
		{
			in: `$(call func, \
	foo)`,
			val: &funcCall{
				fclosure: fclosure{
					args: []Value{
						literal("(call"),
						literal("func"),
						literal(" foo"),
					},
				},
			},
		},
		{
			in: `$(call func, \)`,
			val: &funcCall{
				fclosure: fclosure{
					args: []Value{
						literal("(call"),
						literal("func"),
						literal(` \`),
					},
				},
			},
		},
		{
			in: `$(eval ## comment)`,
			val: &funcNop{
				expr: `$(eval ## comment)`,
			},
		},
		{
			in: `$(eval foo = bar)`,
			val: &funcEvalAssign{
				lhs: "foo",
				op:  "=",
				rhs: literal("bar"),
			},
		},
		{
			in: `$(eval foo :=)`,
			val: &funcEvalAssign{
				lhs: "foo",
				op:  ":=",
				rhs: literal(""),
			},
		},
		{
			in: `$(eval foo := $(bar))`,
			val: &funcEvalAssign{
				lhs: "foo",
				op:  ":=",
				rhs: &varref{
					varname: literal("bar"),
					paren:   '(',
				},
			},
		},
		{
			in: `$(eval foo := $$(bar))`,
			val: &funcEvalAssign{
				lhs: "foo",
				op:  ":=",
				rhs: literal("$(bar)"),
			},
		},
		{
			in: `$(strip $1)`,
			val: &funcStrip{
				fclosure: fclosure{
					args: []Value{
						literal("(strip"),
						paramref(1),
					},
				},
			},
		},
		{
			in: `$(strip $(1))`,
			val: &funcStrip{
				fclosure: fclosure{
					args: []Value{
						literal("(strip"),
						paramref(1),
					},
				},
			},
		},
	} {
		val, _, err := parseExpr([]byte(tc.in), nil, parseOp{alloc: true})
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
		if got, want := val, tc.val; !reflect.DeepEqual(got, want) {
			t.Errorf("parseExpr(%[1]q)=%[2]q %#[2]v, _, _;\n want %[3]q %#[3]v, _, _", tc.in, got, want)
		}
	}
}
