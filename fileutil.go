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
	"bytes"
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

func existsInVPATH(ev *Evaluator, target string) (string, bool) {
	if exists(target) {
		return target, true
	}
	vpath, found := ev.vars["VPATH"]
	if !found {
		return target, false
	}
	// TODO(ukai): support vpath directive (pattern vpath).
	// TODO(ukai): ok to cache vpath value?
	wb := newWbuf()
	err := vpath.Eval(wb, ev)
	if err != nil {
		return target, false
	}
	// In the 'VPATH' variable, directory names are separated by colons
	// or blanks. (on windows, semi-colons)
	for _, word := range wb.words {
		for _, dir := range bytes.Split(word, []byte{':'}) {
			vtarget := filepath.Join(string(dir), target)
			if exists(vtarget) {
				return vtarget, true
			}
		}
	}
	wb.release()
	return target, false
}
