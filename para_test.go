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
	"testing"
)

func TestPara(t *testing.T) {
	cwd, err := filepath.Abs(".")
	if err != nil {
		panic(err)
	}
	paraPath := filepath.Join(cwd, "para")
	numJobs := 4

	paraChan := make(chan *paraResult)
	para := newParaWorker(paraChan, numJobs, paraPath)
	go para.Run()

	numTasks := 100
	for i := 0; i < numTasks; i++ {
		runners := []runner{
			{
				output: fmt.Sprintf("%d", i),
				cmd:    fmt.Sprintf("echo test%d 2>&1", i),
				shell:  "/bin/sh",
			},
		}
		para.RunCommand(runners)
	}

	var started []*paraResult
	var results []*paraResult
	for len(started) != numTasks || len(results) != numTasks {
		select {
		case r := <-paraChan:
			t.Logf("started=%d finished=%d", len(started), len(results))
			if r.status < 0 && r.signal < 0 {
				started = append(started, r)
			} else {
				results = append(results, r)
			}
		}
		if len(started) < len(results) {
			t.Errorf("got more results than started. started=%d finished=%d", len(started), len(results))
		}
	}

	para.Wait()
}
