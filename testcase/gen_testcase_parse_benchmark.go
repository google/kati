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

const preamble = `package main

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
		ParseMakefileString(mk, {{.Filename | printf "%q"}}, 0)
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
