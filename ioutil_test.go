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

func TestWordBuffer(t *testing.T) {
	for _, tc := range []struct {
		in   []string
		want []string
	}{
		{
			in:   []string{"foo"},
			want: []string{"foo"},
		},
		{
			in:   []string{"foo bar"},
			want: []string{"foo", "bar"},
		},
		{
			in:   []string{"  foo bar\tbaz "},
			want: []string{"foo", "bar", "baz"},
		},
		{
			in:   []string{"foo", "bar"},
			want: []string{"foobar"},
		},
		{
			in:   []string{"foo ", "bar"},
			want: []string{"foo", "bar"},
		},
		{
			in:   []string{"foo", " bar"},
			want: []string{"foo", "bar"},
		},
		{
			in:   []string{"foo ", " bar"},
			want: []string{"foo", "bar"},
		},
	} {
		var wb wordBuffer
		for _, s := range tc.in {
			wb.Write([]byte(s))
		}

		var got []string
		for _, word := range wb.words {
			got = append(got, string(word))
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q => %q; want %q", tc.in, got, tc.want)
		}
	}
}
