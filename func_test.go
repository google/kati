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

package main

import "testing"

func TestRot13(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{
			in:   "PRODUCT_PACKAGE_OVERLAYS",
			want: "CEBQHPG_CNPXNTR_BIREYNLF",
		},
		{
			in:   "product_name",
			want: "cebqhpg_anzr",
		},
	} {
		buf := []byte(tc.in)
		rot13(buf)
		if got, want := string(buf), tc.want; got != want {
			t.Errorf("rot13(%q) got=%q; want=%q", tc.in, got, want)
		}
	}
}
