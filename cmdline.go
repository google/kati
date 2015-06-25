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
	"strings"
)

func ParseCommandLine(cmdline []string) ([]string, []string) {
	var vars []string
	var targets []string
	for _, arg := range cmdline {
		if strings.IndexByte(arg, '=') >= 0 {
			vars = append(vars, arg)
			continue
		}
		targets = append(targets, arg)
	}
	return vars, targets
}

func initVars(vars Vars, kvlist []string, origin string) error {
	for _, v := range kvlist {
		kv := strings.SplitN(v, "=", 2)
		Logf("%s var %q", origin, v)
		if len(kv) < 2 {
			return fmt.Errorf("A weird %s variable %q", origin, kv)
		}
		vars.Assign(kv[0], &recursiveVar{
			expr:   literal(kv[1]),
			origin: origin,
		})
	}
	return nil
}
