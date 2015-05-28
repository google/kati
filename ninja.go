package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type NinjaGenerator struct {
	f      *os.File
	nodes  []*DepNode
	vars   Vars
	ex     *Executor
	ruleId int
	done   map[string]bool
}

func NewNinjaGenerator(g *DepGraph) *NinjaGenerator {
	f, err := os.Create("build.ninja")
	if err != nil {
		panic(err)
	}
	return &NinjaGenerator{
		f:     f,
		nodes: g.nodes,
		vars:  g.vars,
		done:  make(map[string]bool),
	}
}

func getDepfile(ss string) (string, error) {
	tss := ss + " "
	if !strings.Contains(tss, " -MD ") && !strings.Contains(tss, " -MMD ") {
		return "", nil
	}

	// A hack for Android - llvm-rs-cc seems not to emit a dep file.
	if strings.Contains(ss, "bin/llvm-rs-cc ") {
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

		// A hack for Android to get .P files instead of .d.
		p := stripExt(mf) + ".P"
		if strings.Contains(ss, p) {
			return p, nil
		}

		// A hack for Android. For .s files, GCC does not use
		// C preprocessor, so it ignores -MF flag.
		as := "/" + stripExt(filepath.Base(mf)) + ".s"
		if strings.Contains(ss, as) {
			return "", nil
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

func stripShellComment(s string) string {
	if strings.IndexByte(s, '#') < 0 {
		// Fast path.
		return s
	}
	var escape bool
	var quote rune
	for i, c := range s {
		if quote > 0 {
			if quote == c && (quote == '\'' || !escape) {
				quote = 0
			}
		} else if !escape {
			if c == '#' {
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
	}
	return s
}

func genShellScript(runners []runner) string {
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
		buf.WriteString(cmd)
		if i == len(runners)-1 && r.ignoreError {
			buf.WriteString(" ; true")
		}
	}
	return buf.String()
}

func (n *NinjaGenerator) genRuleName() string {
	ruleName := fmt.Sprintf("rule%d", n.ruleId)
	n.ruleId++
	return ruleName
}

func (n *NinjaGenerator) emitBuild(output, rule, dep string) {
	fmt.Fprintf(n.f, "build %s: %s", output, rule)
	if dep != "" {
		fmt.Fprintf(n.f, " %s", dep)
	}
	fmt.Fprintf(n.f, "\n")
}

func getDepString(node *DepNode) string {
	var deps []string
	var orderOnlys []string
	for _, d := range node.Deps {
		if d.IsOrderOnly {
			orderOnlys = append(orderOnlys, d.Output)
		} else {
			deps = append(deps, d.Output)
		}
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

func (n *NinjaGenerator) emitNode(node *DepNode) {
	if n.done[node.Output] {
		return
	}
	n.done[node.Output] = true

	if len(node.Cmds) == 0 && len(node.Deps) == 0 && !node.IsPhony {
		return
	}

	runners, _ := n.ex.createRunners(node, true)
	ruleName := "phony"
	if len(runners) > 0 {
		ruleName = n.genRuleName()
		fmt.Fprintf(n.f, "rule %s\n", ruleName)
		fmt.Fprintf(n.f, " description = build $out\n")

		ss := genShellScript(runners)
		depfile, err := getDepfile(ss)
		if err != nil {
			panic(err)
		}
		if depfile != "" {
			fmt.Fprintf(n.f, " depfile = %s\n", depfile)
		}
		// It seems Linux is OK with ~130kB.
		// TODO: Find this number automatically.
		ArgLenLimit := 100 * 1000
		if len(ss) > ArgLenLimit {
			fmt.Fprintf(n.f, " rspfile = $out.rsp\n")
			fmt.Fprintf(n.f, " rspfile_content = %s\n", ss)
			ss = "sh $out.rsp"
		}
		fmt.Fprintf(n.f, " command = %s\n", ss)

	}
	n.emitBuild(node.Output, ruleName, getDepString(node))

	for _, d := range node.Deps {
		n.emitNode(d)
	}
}

func (n *NinjaGenerator) run() {
	n.ex = NewExecutor(n.vars)
	for _, node := range n.nodes {
		n.emitNode(node)
	}
	n.f.Close()
}

func GenerateNinja(g *DepGraph) {
	NewNinjaGenerator(g).run()
}
