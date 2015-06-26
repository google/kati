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

// gen_testcase_parse_benchmark is a program to generate benchmark tests
// for parsing testcases.
//
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

const preamble = `package kati

import (
	"io/ioutil"
	"testing"
)

`

var tmpl = template.Must(template.New("benchmarktest").Parse(`

func BenchmarkTestcaseParse{{.Name}}(b *testing.B) {
	data, err := ioutil.ReadFile({{.Filename | printf "%q"}})
	if err != nil {
		b.Fatal(err)
	}
	mk := string(data)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseMakefileString(mk, srcpos{
			filename: {{.Filename | printf "%q"}},
			lineno: 0,
		})
	}
}
`))

func testName(fname string) string {
	base := filepath.Base(fname)
	i := strings.Index(base, ".")
	if i >= 0 {
		base = base[:i]
	}
	base = strings.Replace(base, "-", "", -1)
	tn := strings.Title(base)
	return tn
}

func writeBenchmarkTest(w io.Writer, fname string) {
	name := testName(fname)
	if strings.HasPrefix(name, "Err") {
		return
	}
	err := tmpl.Execute(w, struct {
		Name     string
		Filename string
	}{
		Name:     testName(fname),
		Filename: fname,
	})
	if err != nil {
		panic(err)
	}
}

func main() {
	f, err := os.Create("testcase_parse_benchmark_test.go")
	if err != nil {
		panic(err)
	}
	defer func() {
		err := f.Close()
		if err != nil {
			panic(err)
		}
	}()
	fmt.Fprint(f, preamble)
	matches, err := filepath.Glob("testcase/*.mk")
	if err != nil {
		panic(err)
	}
	for _, tc := range matches {
		writeBenchmarkTest(f, tc)
	}
}
