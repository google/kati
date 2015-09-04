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
	"fmt"
	"path/filepath"
	"strings"
)

const bootstrapMakefileName = "*bootstrap*"

func bootstrapMakefile(targets []string) (makefile, error) {
	bootstrap := `
CC?=cc
CXX?=g++
AR?=ar
MAKE?=kati
# Pretend to be GNU make 3.81, for compatibility.
MAKE_VERSION?=3.81
KATI?=kati
SHELL=/bin/sh
# TODO: Add more builtin vars.

# http://www.gnu.org/software/make/manual/make.html#Catalogue-of-Rules
# The document above is actually not correct. See default.c:
# http://git.savannah.gnu.org/cgit/make.git/tree/default.c?id=4.1
.c.o:
	$(CC) $(CFLAGS) $(CPPFLAGS) $(TARGET_ARCH) -c -o $@ $<
.cc.o:
	$(CXX) $(CXXFLAGS) $(CPPFLAGS) $(TARGET_ARCH) -c -o $@ $<
# TODO: Add more builtin rules.
`
	bootstrap += fmt.Sprintf("MAKECMDGOALS:=%s\n", strings.Join(targets, " "))
	cwd, err := filepath.Abs(".")
	if err != nil {
		return makefile{}, err
	}
	bootstrap += fmt.Sprintf("CURDIR:=%s\n", cwd)
	return parseMakefileString(bootstrap, srcpos{bootstrapMakefileName, 0})
}
