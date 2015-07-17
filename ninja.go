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
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

// NinjaGenerator generates ninja build files from DepGraph.
type NinjaGenerator struct {
	// GomaDir is goma directory.  If empty, goma will not be used.
	GomaDir string

	f       *os.File
	nodes   []*DepNode
	exports map[string]bool

	ctx *execContext

	ruleID     int
	done       map[string]bool
	shortNames map[string][]string
}

func (n *NinjaGenerator) init(g *DepGraph) {
	n.nodes = g.nodes
	n.exports = g.exports
	n.ctx = newExecContext(g.vars, g.vpaths, true)
	n.done = make(map[string]bool)
	n.shortNames = make(map[string][]string)
}

func getDepfileImpl(ss string) (string, error) {
	tss := ss + " "
	if !strings.Contains(tss, " -MD ") && !strings.Contains(tss, " -MMD ") {
		return "", nil
	}

	mfIndex := strings.Index(ss, " -MF ")
	if mfIndex >= 0 {
		mf := trimLeftSpace(ss[mfIndex+4:])
		if strings.Index(mf, " -MF ") >= 0 {
			return "", fmt.Errorf("Multiple output file candidates in %s", ss)
		}
		mfEndIndex := strings.IndexAny(mf, " \t\n")
		if mfEndIndex >= 0 {
			mf = mf[:mfEndIndex]
		}

		return mf, nil
	}

	outIndex := strings.Index(ss, " -o ")
	if outIndex < 0 {
		return "", fmt.Errorf("Cannot find the depfile in %s", ss)
	}
	out := trimLeftSpace(ss[outIndex+4:])
	if strings.Index(out, " -o ") >= 0 {
		return "", fmt.Errorf("Multiple output file candidates in %s", ss)
	}
	outEndIndex := strings.IndexAny(out, " \t\n")
	if outEndIndex >= 0 {
		out = out[:outEndIndex]
	}
	return stripExt(out) + ".d", nil
}

// getDepfile gets depfile from cmdline, and returns cmdline and depfile.
func getDepfile(cmdline string) (string, string, error) {
	// A hack for Android - llvm-rs-cc seems not to emit a dep file.
	if strings.Contains(cmdline, "bin/llvm-rs-cc ") {
		return cmdline, "", nil
	}

	depfile, err := getDepfileImpl(cmdline)
	if depfile == "" || err != nil {
		return cmdline, depfile, err
	}

	// A hack for Makefiles generated by automake.
	mvCmd := "(mv -f " + depfile + " "
	if i := strings.LastIndex(cmdline, mvCmd); i >= 0 {
		rest := cmdline[i+len(mvCmd):]
		ei := strings.IndexByte(rest, ')')
		if ei < 0 {
			return cmdline, "", fmt.Errorf("unbalanced parenthes? %s", cmdline)
		}
		cmdline = cmdline[:i] + "(cp -f " + depfile + " " + rest
		return cmdline, depfile, nil
	}

	// A hack for Android to get .P files instead of .d.
	p := stripExt(depfile) + ".P"
	if strings.Contains(cmdline, p) {
		rmfCmd := "; rm -f " + depfile
		ncmdline := strings.Replace(cmdline, rmfCmd, "", 1)
		if ncmdline == cmdline {
			return cmdline, "", fmt.Errorf("cannot find removal of .d file: %s", cmdline)
		}
		return ncmdline, p, nil
	}

	// A hack for Android. For .s files, GCC does not use
	// C preprocessor, so it ignores -MF flag.
	as := "/" + stripExt(filepath.Base(depfile)) + ".s"
	if strings.Contains(cmdline, as) {
		return cmdline, "", nil
	}

	cmdline += fmt.Sprintf(" && cp %s %s.tmp", depfile, depfile)
	depfile += ".tmp"
	return cmdline, depfile, nil
}

func stripShellComment(s string) string {
	if strings.IndexByte(s, '#') < 0 {
		// Fast path.
		return s
	}
	// set space as an initial value so the leading comment will be
	// stripped out.
	lastch := rune(' ')
	var escape bool
	var quote rune
	for i, c := range s {
		if quote > 0 {
			if quote == c && (quote == '\'' || !escape) {
				quote = 0
			}
		} else if !escape {
			if c == '#' && isWhitespace(lastch) {
				return s[:i]
			} else if c == '\'' || c == '"' || c == '`' {
				quote = c
			}
		}
		if escape {
			escape = false
		} else if c == '\\' {
			escape = true
		} else {
			escape = false
		}
		lastch = c
	}
	return s
}

var ccRE = regexp.MustCompile(`^prebuilts/(gcc|clang)/.*(gcc|g\+\+|clang|clang\+\+) .* ?-c `)

func gomaCmdForAndroidCompileCmd(cmd string) (string, bool) {
	i := strings.Index(cmd, " ")
	if i < 0 {
		return cmd, false
	}
	driver := cmd[:i]
	if strings.HasSuffix(driver, "ccache") {
		return gomaCmdForAndroidCompileCmd(cmd[i+1:])
	}
	return cmd, ccRE.MatchString(cmd)
}

func (n *NinjaGenerator) genShellScript(runners []runner) (string, bool) {
	useGomacc := false
	var buf bytes.Buffer
	for i, r := range runners {
		if i > 0 {
			if runners[i-1].ignoreError {
				buf.WriteString(" ; ")
			} else {
				buf.WriteString(" && ")
			}
		}
		cmd := stripShellComment(r.cmd)
		cmd = trimLeftSpace(cmd)
		cmd = strings.Replace(cmd, "\\\n", " ", -1)
		cmd = strings.TrimRight(cmd, " \t\n;")
		cmd = strings.Replace(cmd, "$", "$$", -1)
		cmd = strings.Replace(cmd, "\t", " ", -1)
		if cmd == "" {
			cmd = "true"
		}
		if n.GomaDir != "" {
			rcmd, ok := gomaCmdForAndroidCompileCmd(cmd)
			if ok {
				cmd = fmt.Sprintf("%s/gomacc %s", n.GomaDir, rcmd)
				useGomacc = true
			}
		}

		needsSubShell := i > 0 || len(runners) > 1
		if cmd[0] == '(' {
			needsSubShell = false
		}

		if needsSubShell {
			buf.WriteByte('(')
		}
		buf.WriteString(cmd)
		if i == len(runners)-1 && r.ignoreError {
			buf.WriteString(" ; true")
		}
		if needsSubShell {
			buf.WriteByte(')')
		}
	}
	return buf.String(), n.GomaDir != "" && !useGomacc
}

func (n *NinjaGenerator) genRuleName() string {
	ruleName := fmt.Sprintf("rule%d", n.ruleID)
	n.ruleID++
	return ruleName
}

func (n *NinjaGenerator) emitBuild(output, rule, dep string) {
	fmt.Fprintf(n.f, "build %s: %s%s\n", output, rule, dep)
}

func getDepString(node *DepNode) string {
	var deps []string
	for _, d := range node.Deps {
		deps = append(deps, d.Output)
	}
	var orderOnlys []string
	for _, d := range node.OrderOnlys {
		orderOnlys = append(orderOnlys, d.Output)
	}
	dep := ""
	if len(deps) > 0 {
		dep += fmt.Sprintf(" %s", strings.Join(deps, " "))
	}
	if len(orderOnlys) > 0 {
		dep += fmt.Sprintf(" || %s", strings.Join(orderOnlys, " "))
	}
	return dep
}

func (n *NinjaGenerator) emitNode(node *DepNode) error {
	if n.done[node.Output] {
		return nil
	}
	n.done[node.Output] = true

	if len(node.Cmds) == 0 && len(node.Deps) == 0 && len(node.OrderOnlys) == 0 && !node.IsPhony {
		if _, ok := n.ctx.vpaths.exists(node.Output); ok {
			return nil
		}
		n.emitBuild(node.Output, "phony", "")
		fmt.Fprintln(n.f)
		return nil
	}

	base := filepath.Base(node.Output)
	if base != node.Output {
		n.shortNames[base] = append(n.shortNames[base], node.Output)
	}

	runners, _, err := createRunners(n.ctx, node)
	if err != nil {
		return err
	}
	ruleName := "phony"
	useLocalPool := false
	if len(runners) > 0 {
		ruleName = n.genRuleName()
		fmt.Fprintf(n.f, "\n# rule for %s\n", node.Output)
		fmt.Fprintf(n.f, "rule %s\n", ruleName)
		fmt.Fprintf(n.f, " description = build $out\n")

		ss, ulp := n.genShellScript(runners)
		if ulp {
			useLocalPool = true
		}
		cmdline, depfile, err := getDepfile(ss)
		if err != nil {
			return err
		}
		if depfile != "" {
			fmt.Fprintf(n.f, " depfile = %s\n", depfile)
			fmt.Fprintf(n.f, " deps = gcc\n")
		}
		// It seems Linux is OK with ~130kB.
		// TODO: Find this number automatically.
		ArgLenLimit := 100 * 1000
		if len(ss) > ArgLenLimit {
			fmt.Fprintf(n.f, " rspfile = $out.rsp\n")
			fmt.Fprintf(n.f, " rspfile_content = %s\n", ss)
			ss = "sh $out.rsp"
		}
		fmt.Fprintf(n.f, " command = %s\n", cmdline)

	}
	n.emitBuild(node.Output, ruleName, getDepString(node))
	if useLocalPool {
		fmt.Fprintf(n.f, " pool = local_pool\n")
	}
	fmt.Fprintf(n.f, "\n")

	for _, d := range node.Deps {
		err := n.emitNode(d)
		if err != nil {
			return err
		}
	}
	for _, d := range node.OrderOnlys {
		err := n.emitNode(d)
		if err != nil {
			return err
		}
	}
	return nil
}

func (n *NinjaGenerator) generateShell(suffix string) (err error) {
	f, err := os.Create(fmt.Sprintf("ninja%s.sh", suffix))
	if err != nil {
		return err
	}
	defer func() {
		cerr := f.Close()
		if err == nil {
			err = cerr
		}
	}()

	fmt.Fprintf(f, "#!%s\n", n.ctx.shell)
	fmt.Fprintf(f, "# Generated by kati %s\n", gitVersion)
	fmt.Fprintln(f)
	fmt.Fprintln(f, `cd $(dirname "$0")`)
	for name, export := range n.exports {
		if export {
			v, err := n.ctx.ev.EvaluateVar(name)
			if err != nil {
				return err
			}
			fmt.Fprintf(f, "export %s=%s\n", name, v)
		} else {
			fmt.Fprintf(f, "unset %s\n", name)
		}
	}
	if n.GomaDir == "" {
		fmt.Fprintln(f, `exec ninja "$@"`)
	} else {
		fmt.Fprintln(f, `exec ninja -j300 "$@"`)
	}

	return f.Chmod(0755)
}

func (n *NinjaGenerator) generateNinja(suffix, defaultTarget string) (err error) {
	f, err := os.Create(fmt.Sprintf("build%s.ninja", suffix))
	if err != nil {
		return err
	}
	defer func() {
		cerr := f.Close()
		if err == nil {
			err = cerr
		}
	}()

	n.f = f
	fmt.Fprintf(n.f, "# Generated by kati %s\n", gitVersion)
	fmt.Fprintf(n.f, "\n")

	if len(usedEnvs) > 0 {
		fmt.Fprintln(n.f, "# Environment variables used:")
		var names []string
		for name := range usedEnvs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			v, err := n.ctx.ev.EvaluateVar(name)
			if err != nil {
				return err
			}
			fmt.Fprintf(n.f, "# %s=%s\n", name, v)
		}
		fmt.Fprintf(n.f, "\n")
	}

	if n.GomaDir != "" {
		fmt.Fprintf(n.f, "pool local_pool\n")
		fmt.Fprintf(n.f, " depth = %d\n", runtime.NumCPU())
	}

	for _, node := range n.nodes {
		err := n.emitNode(node)
		if err != nil {
			return err
		}
	}

	if defaultTarget != "" {
		fmt.Fprintf(n.f, "\ndefault %s\n", defaultTarget)
	}

	fmt.Fprintf(n.f, "\n# shortcuts:\n")
	var names []string
	for name := range n.shortNames {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if len(n.shortNames[name]) != 1 {
			// we generate shortcuts only for targets whose basename are unique.
			continue
		}
		fmt.Fprintf(n.f, "build %s: phony %s\n", name, n.shortNames[name][0])
	}
	return nil
}

// Save generates build.ninja from DepGraph.
func (n *NinjaGenerator) Save(g *DepGraph, suffix string, targets []string) error {
	startTime := time.Now()
	n.init(g)
	err := n.generateShell(suffix)
	if err != nil {
		return err
	}
	var defaultTarget string
	if len(targets) == 0 && len(g.nodes) > 0 {
		defaultTarget = g.nodes[0].Output
	}
	err = n.generateNinja(suffix, defaultTarget)
	if err != nil {
		return err
	}
	logStats("generate ninja time: %q", time.Since(startTime))
	return nil
}
