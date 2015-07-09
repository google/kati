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

import "testing"

func BenchmarkFuncStrip(b *testing.B) {
	strip := &funcStrip{
		fclosure: fclosure{
			args: []Value{
				literal("(strip"),
				literal("a b  c "),
			},
		},
	}
	ev := NewEvaluator(make(map[string]Var))
	var buf evalBuffer
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		strip.Eval(&buf, ev)
	}
}

func BenchmarkFuncSort(b *testing.B) {
	sort := &funcSort{
		fclosure: fclosure{
			args: []Value{
				literal("(sort"),
				literal("foo bar lose"),
			},
		},
	}
	ev := NewEvaluator(make(map[string]Var))
	var buf evalBuffer
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		sort.Eval(&buf, ev)
	}
}

func BenchmarkFuncPatsubst(b *testing.B) {
	patsubst := &funcPatsubst{
		fclosure: fclosure{
			args: []Value{
				literal("(patsubst"),
				literal("%.java"),
				literal("%.class"),
				literal("foo.jar bar.java baz.h"),
			},
		},
	}
	ev := NewEvaluator(make(map[string]Var))
	var buf evalBuffer
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		patsubst.Eval(&buf, ev)
	}
}
