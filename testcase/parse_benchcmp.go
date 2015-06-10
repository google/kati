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

/*
Program parse_benchcmp runs testcase_parse_benchmark and displays
performance changes.

*/
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func run(prog string, args ...string) {
	cmd := exec.Command(prog, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		panic(err)
	}
}

func output(prog string, args ...string) string {
	cmd := exec.Command(prog, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		panic(err)
	}
	return strings.TrimSpace(string(out))
}

func runBenchtest(fname string) {
	run("go", "generate")
	f, err := os.Create(fname)
	if err != nil {
		panic(err)
	}
	defer func() {
		err = f.Close()
		if err != nil {
			panic(err)
		}
	}()
	cmd := exec.Command("go", "test", "-run", "NONE", "-bench", ".")
	cmd.Stdout = f
	err = cmd.Run()
	if err != nil {
		panic(err)
	}
}

func main() {
	_, err := exec.LookPath("benchcmp")
	if err != nil {
		fmt.Fprintln(os.Stderr, "benchcmp not found:", err)
		fmt.Fprintln(os.Stderr, "install it by:")
		fmt.Fprintln(os.Stderr, " export GOPATH=$HOME  # if not set")
		fmt.Fprintln(os.Stderr, " PATH=$PATH:$GOPATH/bin")
		fmt.Fprintln(os.Stderr, " go get -u golang.org/x/tools/cmd/benchcmp")
		os.Exit(1)
	}
	status := output("git", "status", "-s")
	if status != "" {
		fmt.Fprintln(os.Stderr, "workspace is dirty. please commit.")
		fmt.Fprintln(os.Stderr, status)
		os.Exit(1)
	}
	curBranch := output("git", "symbolic-ref", "--short", "HEAD")
	if curBranch == "master" {
		fmt.Fprintln(os.Stderr, "current branch is master.")
		fmt.Fprintln(os.Stderr, "run in branch to compare with master.")
		os.Exit(1)
	}
	fmt.Println("Run benchmark on master and ", curBranch)
	fmt.Println("git checkout master")
	run("git", "checkout", "master")
	run("git", "clean", "-f")
	commit := output("git", "log", "--oneline", "-1")
	fmt.Println(commit)
	fmt.Println("running benchmark tests...")
	runBenchtest("bench-old.out")

	fmt.Println("git checkout", curBranch)
	run("git", "checkout", curBranch)
	run("git", "clean", "-f")
	commit = output("git", "log", "--oneline", "-1")
	fmt.Println(commit)
	fmt.Println("running benchmark tests...")
	runBenchtest("bench-new.out")

	run("benchcmp", "bench-old.out", "bench-new.out")
}
