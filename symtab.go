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

import "sync"

type symtabT struct {
	mu sync.Mutex
	m  map[string]string
}

var symtab = &symtabT{
	m: make(map[string]string),
}

func intern(s string) string {
	symtab.mu.Lock()
	v, ok := symtab.m[s]
	if ok {
		symtab.mu.Unlock()
		return v
	}
	symtab.m[s] = s
	symtab.mu.Unlock()
	return s
}

func internBytes(s []byte) string {
	return intern(string(s))
}
