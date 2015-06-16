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

// make-c is simple program to measure time to parse Makefiles in android.
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"time"
)

func main() {
	parseDone := make(chan bool)
	cmd := exec.Command("make", "-n")
	r, err := cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}
	t := time.Now()
	go func() {
		s := bufio.NewScanner(r)
		for s.Scan() {
			if bytes.HasPrefix(s.Bytes(), []byte("echo ")) {
				parseDone <- true
				return
			}
			fmt.Println(s.Text())
		}
		if err := s.Err(); err != nil {
			panic(err)
		}
		panic("unexpected end of make?")
	}()
	err = cmd.Start()
	if err != nil {
		panic(err)
	}
	select {
	case <-parseDone:
		fmt.Printf("make -c: %v\n", time.Since(t))
	}
	cmd.Process.Kill()
	cmd.Wait()
}
