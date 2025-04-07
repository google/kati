// Copyright 2020 Google Inc. All rights reserved
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
	"context"
	"flag"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/sergi/go-diff/diffmatchpatch"
)

var ninja bool
var genAllTargets bool
var rkati bool

func init() {
	// suppress GNU make jobserver magic when calling "make"
	os.Unsetenv("MAKEFLAGS")
	os.Unsetenv("MAKELEVEL")
	os.Setenv("NINJA_STATUS", "NINJACMD: ")

	flag.BoolVar(&ninja, "ninja", false, "use ninja")
	flag.BoolVar(&genAllTargets, "all", false, "use --gen_all_targets")
	flag.BoolVar(&rkati, "rkati", false, "compare rkati against ckati")
}

type normalization struct {
	regexp  *regexp.Regexp
	replace string
}

var normalizeQuotes = normalization{
	regexp.MustCompile("([`'\"]|\xe2\x80\x98|\xe2\x80\x99)"), `"`,
}

var normalizeMakeLog = []normalization{
	normalizeQuotes,
	{regexp.MustCompile(`make(?:\[\d+\])?: (Entering|Leaving) directory[^\n]*\n`), ""},
	{regexp.MustCompile(`make(?:\[\d+\])?: `), ""},

	// Normalizations for old/new GNU make.
	{regexp.MustCompile(" recipe for target "), " commands for target "},
	{regexp.MustCompile(" recipe commences "), " commands commence "},
	{regexp.MustCompile("missing rule before recipe."), "missing rule before commands."},
	{regexp.MustCompile(" (did you mean TAB instead of 8 spaces?)"), ""},
	{regexp.MustCompile("Extraneous text after"), "extraneous text after"},
	// Not sure if this is useful
	{regexp.MustCompile(`\s+Stop\.`), ""},
	// GNU make 4.0 has this output.
	{regexp.MustCompile(`Makefile:\d+: commands for target ".*?" failed\n`), ""},
	// We treat some warnings as errors.
	{regexp.MustCompile(`/bin/(ba)?sh: line 1: `), ""},
	// Normalization for "include foo" with C++ kati
	{regexp.MustCompile(`(: \S+: No such file or directory)\n\*\*\* No rule to make target "[^"]+".`), "$1"},
	// GNU make 4.0 prints the file:line as part of the error message, e.g.:
	//    *** [Makefile:4: target] Error 1
	{regexp.MustCompile(`\[\S+:\d+: `), "["},
}

var normalizeMakeNinja = normalization{
	// We print out some ninja warnings in some tests to match what we expect
	// ninja to produce. Remove them if we're not testing ninja
	regexp.MustCompile("ninja: warning: [^\n]+"), "",
}

var normalizeKati = []normalization{
	normalizeQuotes,

	// kati specific log messages
	{regexp.MustCompile(`\*kati\*[^\n]*`), ""},
	{regexp.MustCompile(`c?kati: `), ""},
	{regexp.MustCompile(`/bin/(ba)?sh: line 1: `), ""},
	{regexp.MustCompile(`/bin/sh: `), ""},
	{regexp.MustCompile(`.*: warning for parse error in an unevaluated line: [^\n]*`), ""},
	{regexp.MustCompile(`([^\n ]+: )?FindEmulator: `), ""},
	// kati log ifles in find_command.mk
	{regexp.MustCompile(` (\./+)+kati\.\S+`), ""},
	// json files in find_command.mk
	{regexp.MustCompile(` (\./+)+test\S+.json`), ""},
	// Normalization for "include foo" with Go kati
	{regexp.MustCompile(`(: )open (\S+): n(o such file or directory)\nNOTE:[^\n]*`), "${1}${2}: N${3}"},
	// Bionic libc has different error messages than glibc
	{regexp.MustCompile(`Too many symbolic links encountered`), "Too many levels of symbolic links"},
	// Rust includes the numeric error code in I/O errors
	{regexp.MustCompile(` \(os error \d+\)`), ""},
}

var normalizeNinja = []normalization{
	{regexp.MustCompile(`NINJACMD: [^\n]*\n`), ""},
	{regexp.MustCompile(`ninja: no work to do\.\n`), ""},
	{regexp.MustCompile(`ninja: error: (.*, needed by .*),[^\n]*`),
		"*** No rule to make target ${1}."},
	{regexp.MustCompile(`ninja: warning: multiple rules generate (.*)\. builds involving this target will not be correct[^\n]*`),
		"ninja: warning: multiple rules generate ${1}."},
}

var normalizeNinjaFail = []normalization{
	{regexp.MustCompile(`FAILED: ([^\n]+\n/bin/bash)?[^\n]*\n`), "*** [test] Error 1\n"},
	{regexp.MustCompile(`ninja: [^\n]+\n`), ""},
}

var normalizeNinjaIgnoreFail = []normalization{
	{regexp.MustCompile(`FAILED: ([^\n]+\n/bin/bash)?[^\n]*\n`), ""},
	{regexp.MustCompile(`ninja: [^\n]+\n`), ""},
}

var circularRE = regexp.MustCompile(`(Circular .* dropped\.\n)`)

func normalize(log []byte, normalizations []normalization) []byte {
	// We don't care when circular dependency detection happens.
	ret := []byte{}
	for _, circ := range circularRE.FindAllSubmatch(log, -1) {
		ret = append(ret, circ[1]...)
	}
	ret = append(ret, circularRE.ReplaceAll(log, []byte{})...)

	for _, n := range normalizations {
		ret = n.regexp.ReplaceAll(ret, []byte(n.replace))
	}
	return ret
}

func runMake(t *testing.T, prefix []string, dir string, silent bool, tc string) string {
	write := func(f string, data []byte) {
		suffix := ""
		if tc != "" {
			suffix = "_" + tc
		}
		if err := ioutil.WriteFile(filepath.Join(dir, f+suffix), data, 0666); err != nil {
			t.Error(err)
		}
	}

	args := append(prefix, "make")
	if silent {
		args = append(args, "-s")
	}
	if tc != "" {
		args = append(args, tc)
	}
	args = append(args, "SHELL=/bin/bash")

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	output, _ := cmd.CombinedOutput()
	write("stdout", output)

	output = normalize(output, normalizeMakeLog)
	if !ninja {
		output = normalize(output, []normalization{normalizeMakeNinja})
	}

	write("stdout_normalized", output)
	return string(output)
}

func runKati(t *testing.T, test, dir string, silent bool, rust bool, tc string) string {
	write := func(f string, data []byte) {
		suffix := ""
		if tc != "" {
			suffix = "_" + tc
		}
		if err := ioutil.WriteFile(filepath.Join(dir, f+suffix), data, 0666); err != nil {
			t.Error(err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var kati string
	if rust {
		kati = "../../../target/debug/rkati"
	} else {
		kati = "../../../ckati"
	}
	cmd := exec.CommandContext(ctx, kati, "--use_find_emulator")
	if ninja {
		cmd.Args = append(cmd.Args, "--ninja")
	}
	if genAllTargets {
		cmd.Args = append(cmd.Args, "--gen_all_targets")
	}
	if silent {
		cmd.Args = append(cmd.Args, "-s")
	}
	cmd.Args = append(cmd.Args, "SHELL=/bin/bash")
	if tc != "" && (!genAllTargets || strings.Contains(test, "makecmdgoals")) {
		cmd.Args = append(cmd.Args, tc)
	}
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	write("stdout", output)
	if err != nil {
		output := normalize(output, normalizeKati)
		write("stdout_normalized", output)
		return string(output)
	}

	if ninja {
		ninjaCmd := exec.Command("./ninja.sh", "-j1", "-v")
		if genAllTargets && tc != "" {
			ninjaCmd.Args = append(ninjaCmd.Args, tc)
		}
		ninjaCmd.Dir = dir
		ninjaOutput, _ := ninjaCmd.CombinedOutput()
		write("stdout_ninja", ninjaOutput)
		ninjaOutput = normalize(ninjaOutput, normalizeNinja)
		if test == "err_error_in_recipe.mk" {
			ninjaOutput = normalize(ninjaOutput, normalizeNinjaIgnoreFail)
		} else if strings.HasPrefix(test, "fail_") {
			ninjaOutput = normalize(ninjaOutput, normalizeNinjaFail)
		}
		write("stdout_ninja_normalized", ninjaOutput)
		output = append(output, ninjaOutput...)
	}

	output = normalize(output, normalizeKati)
	write("stdout_normalized", output)
	return string(output)
}

func runKatiInScript(t *testing.T, script, dir string, isNinjaTest bool, rust bool) string {
	write := func(f string, data []byte) {
		if err := ioutil.WriteFile(filepath.Join(dir, f), data, 0666); err != nil {
			t.Error(err)
		}
	}

	var kati string
	if rust {
		kati = "../../../target/debug/rkati"
	} else {
		kati = "../../../ckati"
	}
	args := []string{"bash", script, kati}
	if isNinjaTest {
		args = append(args, "--ninja", "--regen")
	}
	args = append(args, "SHELL=/bin/bash")

	var stderrb bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Stderr = &stderrb
	output, _ := cmd.Output()
	write("stdout", output)
	write("stderr", stderrb.Bytes())
	if isNinjaTest {
		output = normalize(output, normalizeNinja)
	}
	output = normalize(output, normalizeKati)
	write("stdout_normalized", output)
	return string(output)
}

func inList(list []string, item string) bool {
	for _, i := range list {
		if item == i {
			return true
		}
	}
	return false
}

func diffLists(a, b []string) (onlyA []string, onlyB []string) {
	for _, i := range a {
		if !inList(b, i) {
			onlyA = append(onlyA, i)
		}
	}
	for _, i := range b {
		if !inList(a, i) {
			onlyB = append(onlyB, i)
		}
	}
	return
}

func outputFiles(t *testing.T, dir string) []string {
	ret := []string{}
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	ignoreFiles := []string{
		".", "..", "Makefile", "build.ninja", "env.sh", "ninja.sh", "gmon.out", "submake",
	}
	for _, fi := range files {
		name := fi.Name()
		if inList(ignoreFiles, name) ||
			strings.HasPrefix(name, ".") ||
			strings.HasSuffix(name, ".json") ||
			strings.HasPrefix(name, "kati") ||
			strings.HasPrefix(name, "stdout") {
			continue
		}
		ret = append(ret, fi.Name())
	}
	return ret
}

var testcaseRE = regexp.MustCompile(`^test\d*`)

func uniqueTestcases(c []byte) []string {
	seen := map[string]bool{}
	ret := []string{}
	for _, line := range bytes.Split(c, []byte("\n")) {
		line := string(line)
		s := testcaseRE.FindString(line)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = true
		ret = append(ret, s)
	}
	sort.Strings(ret)
	if len(ret) == 0 {
		return []string{""}
	}
	return ret
}

var todoRE = regexp.MustCompile(`^# TODO(?:\(([-a-z|]+)(?:/([-a-z0-9|]+))?\))?`)

func isExpectedFailure(c []byte, tc string) bool {
	for _, line := range bytes.Split(c, []byte("\n")) {
		line := string(line)
		if !strings.HasPrefix(line, "#!") && !strings.HasPrefix(line, "# TODO") {
			break
		}

		todo := todoRE.FindStringSubmatch(line)
		if todo == nil {
			continue
		}

		if todo[1] == "" {
			return true
		}

		todos := strings.Split(todo[1], "|")
		if (inList(todos, "ninja") && ninja) ||
			(inList(todos, "ninja-genall") && ninja && genAllTargets) ||
			(inList(todos, "all")) {

			if todo[2] == "" {
				return true
			}
			tcs := strings.Split(todo[2], "|")
			if inList(tcs, tc) {
				return true
			}
		}
	}
	return false
}

func TestKati(t *testing.T) {
	if _, err := os.Stat("ckati"); err != nil {
		t.Fatalf("ckati must be built before testing: %s", err)
	}
	if rkati {
		if _, err := os.Stat("target/debug/rkati"); err != nil {
			t.Fatalf("rkati must be built before testing: %s", err)
		}
	}
	if ninja {
		if _, err := exec.LookPath("ninja"); err != nil {
			t.Fatal(err)
		}
	}

	out, _ := filepath.Abs("out")
	files, err := ioutil.ReadDir("testcase")
	if err != nil {
		t.Fatal(err)
	}
	for _, fi := range files {
		name := fi.Name()

		isMkTest := strings.HasSuffix(name, ".mk")
		isShTest := strings.HasSuffix(name, ".sh")
		if strings.HasPrefix(name, ".") || !(isMkTest || isShTest) {
			continue
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, err := ioutil.ReadFile(filepath.Join("testcase", name))
			if err != nil {
				t.Fatal(err)
			}

			out := filepath.Join(out, name)
			if err := os.RemoveAll(out); err != nil {
				t.Fatal(err)
			}
			outMake := filepath.Join(out, "make")
			outKati := filepath.Join(out, "kati")
			outRKati := filepath.Join(out, "rkati")
			if err := os.MkdirAll(outKati, 0777); err != nil {
				t.Fatal(err)
			}
			if rkati {
				if err := os.MkdirAll(outRKati, 0777); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.MkdirAll(outMake, 0777); err != nil {
					t.Fatal(err)
				}
			}

			testcases := []string{""}
			expected := map[string]string{}
			expectedFiles := map[string][]string{}
			expectedFailures := map[string]bool{}
			got := map[string]string{}
			gotFiles := map[string][]string{}

			if isMkTest {
				setup := func(dir string) {
					if err = ioutil.WriteFile(filepath.Join(dir, "Makefile"), c, 0666); err != nil {
						t.Fatal(err)
					}
					os.Symlink("../../../testcase/submake", filepath.Join(dir, "submake"))
				}
				setup(outKati)
				if rkati {
					setup(outRKati)
				} else {
					setup(outMake)
				}

				testcases = uniqueTestcases(c)

				isSilent := strings.HasPrefix(name, "submake_")

				for _, tc := range testcases {
					if !rkati {
						expected[tc] = runMake(t, nil, outMake, ninja || isSilent, tc)
						expectedFiles[tc] = outputFiles(t, outMake)
					} else {
						expected[tc] = runKati(t, name, outKati, isSilent, false, tc)
						expectedFiles[tc] = outputFiles(t, outKati)
					}
					expectedFailures[tc] = isExpectedFailure(c, tc)
				}

				for _, tc := range testcases {
					if rkati {
						got[tc] = runKati(t, name, outRKati, isSilent, true, tc)
						gotFiles[tc] = outputFiles(t, outRKati)
					} else {
						got[tc] = runKati(t, name, outKati, isSilent, false, tc)
						gotFiles[tc] = outputFiles(t, outKati)
					}
				}
			} else if isShTest {
				isNinjaTest := strings.HasPrefix(name, "ninja_")
				if isNinjaTest && !ninja {
					t.SkipNow()
				}

				scriptName := "../../../testcase/" + name

				if rkati {
					expected[""] = runKatiInScript(t, scriptName, outKati, isNinjaTest, false)
				} else {
					expected[""] = runMake(t, []string{"bash", scriptName}, outMake, isNinjaTest, "")
				}
				expectedFailures[""] = isExpectedFailure(c, "")

				if rkati {
					got[""] = runKatiInScript(t, scriptName, outRKati, isNinjaTest, true)
				} else {
					got[""] = runKatiInScript(t, scriptName, outKati, isNinjaTest, false)
				}
			}

			check := func(t *testing.T, m, k string, mFiles, kFiles []string, expectFail bool) {
				var a, b string
				if rkati {
					a = "ckati"
					b = "rkati"
				} else {
					a = "Make"
					b = "Kati"
				}

				if !rkati && strings.Contains(m, "FAIL") {
					t.Fatalf("%s returned 'FAIL':\n%q", a, m)
				}

				if !expectFail && m != k {
					dmp := diffmatchpatch.New()
					diffs := dmp.DiffMain(k, m, true)
					diffs = dmp.DiffCleanupSemantic(diffs)
					t.Errorf("Different output from %s (red) to the expected value from %s (green):\n%s",
						b, a, dmp.DiffPrettyText(diffs))
				} else if expectFail && m == k && !rkati {
					t.Errorf("Expected failure, but output is the same")
				}

				if !expectFail {
					onlyA, onlyB := diffLists(mFiles, kFiles)
					if len(onlyA) > 0 {
						t.Errorf("Files only created by %s:\n%q", a, onlyA)
					}
					if len(onlyB) > 0 {
						t.Errorf("Files only created by %s:\n%q", b, onlyB)
					}
				}
			}

			for _, tc := range testcases {
				if tc == "" || len(testcases) == 1 {
					check(t, expected[tc], got[tc], expectedFiles[tc], gotFiles[tc], expectedFailures[tc])
				} else {
					t.Run(tc, func(t *testing.T) {
						check(t, expected[tc], got[tc], expectedFiles[tc], gotFiles[tc], expectedFailures[tc])
					})
				}
			}
		})
	}
}
