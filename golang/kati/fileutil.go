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
	"os"
	"path/filepath"
)

func exists(filename string) bool {
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return true
}

type vpath struct {
	pattern string
	dirs    []string
}

type searchPaths struct {
	vpaths []vpath  // vpath directives
	dirs   []string // VPATH variable
}

func (s searchPaths) exists(target string) (string, bool) {
	if exists(target) {
		return target, true
	}
	for _, vpath := range s.vpaths {
		if !matchPattern(vpath.pattern, target) {
			continue
		}
		for _, dir := range vpath.dirs {
			vtarget := filepath.Join(dir, target)
			if exists(vtarget) {
				return vtarget, true
			}
		}
	}
	for _, dir := range s.dirs {
		vtarget := filepath.Join(dir, target)
		if exists(vtarget) {
			return vtarget, true
		}
	}
	return target, false
}
