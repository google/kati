package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type NinjaTrieNode struct {
	val    string
	parent *NinjaTrieNode
	name   string
}

type NinjaBuild struct {
	output  string
	cmd     string
	deps    string
	depfile string

	index  int
	params []string
	trie   *NinjaTrieNode
}

type NinjaGenerator struct {
	f      *os.File
	nodes  []*DepNode
	vars   Vars
	ex     *Executor
	ruleId int
	done   map[string]bool

	builds []*NinjaBuild
	leafs  []*NinjaTrieNode
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
	fmt.Fprintf(n.f, "build %s: %s%s\n", output, rule, dep)
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

func (n *NinjaGenerator) processNode(node *DepNode) {
	if n.done[node.Output] {
		return
	}
	n.done[node.Output] = true

	if len(node.Cmds) == 0 && len(node.Deps) == 0 && !node.IsPhony {
		return
	}

	runners, _ := n.ex.createRunners(node, true)
	deps := getDepString(node)
	if len(runners) == 0 {
		n.emitBuild(node.Output, "phony", deps)
	} else {
		cmd := genShellScript(runners)
		depfile, err := getDepfile(cmd)
		if err != nil {
			panic(err)
		}

		// It seems Linux is OK with ~130kB.
		// TODO: Find this number automatically.
		ArgLenLimit := 100 * 1000
		if len(cmd) > ArgLenLimit {
			n.emitBuild(node.Output, "longcmd", deps)
			fmt.Fprintf(n.f, " rspfile = $out.rsp\n")
			fmt.Fprintf(n.f, " rspfile_content = %s\n", cmd)
		} else {
			n.builds = append(n.builds, &NinjaBuild{
				output:  node.Output,
				cmd:     genShellScript(runners),
				deps:    deps,
				depfile: depfile,
			})
		}
	}

	for _, d := range node.Deps {
		n.processNode(d)
	}
}

func (b *NinjaBuild) getNextToken() string {
	fs := (b.cmd[b.index] == ' ')
	fi := b.index
	for b.index++; b.index < len(b.cmd); b.index++ {
		if fs != (b.cmd[b.index] == ' ') {
			break
		}
	}
	return b.cmd[fi:b.index]
}

func (n *NinjaGenerator) genParamName(paramIndex int) string {
	return strconv.FormatInt(int64(paramIndex), 36)
}

func (n *NinjaGenerator) constructTrie(builds []*NinjaBuild, node *NinjaTrieNode, paramIndex int) {
	nextsMap := make(map[string][]*NinjaBuild)
	isLeaf := false
	for _, b := range builds {
		if b.index == len(b.cmd) {
			if !isLeaf {
				node.name = n.genRuleName()
				n.leafs = append(n.leafs, node)
				isLeaf = true
			}
			b.trie = node
			continue
		}

		tok := b.getNextToken()
		nextsMap[tok] = append(nextsMap[tok], b)
	}

	var rareChoices []*NinjaBuild
	for tok, nexts := range nextsMap {
		if len(nexts) < len(builds)/10 {
			for _, b := range nexts {
				b.params = append(b.params, tok)
				rareChoices = append(rareChoices, b)
			}
			continue
		}
		nn := &NinjaTrieNode{
			val:    tok,
			parent: node,
		}
		n.constructTrie(nexts, nn, paramIndex)
	}

	if len(rareChoices) == 0 {
		return
	}

	nn := &NinjaTrieNode{
		val:    "$" + n.genParamName(paramIndex),
		parent: node,
	}
	n.constructTrie(rareChoices, nn, paramIndex+1)
}

func (n *NinjaGenerator) run() {
	fmt.Fprintf(n.f, "# Generated by kati\n")
	fmt.Fprintf(n.f, "\n")

	fmt.Fprintf(n.f, "rule longcmd\n")
	fmt.Fprintf(n.f, " command = sh $out.rsp\n")
	fmt.Fprintf(n.f, " description = build $out\n")
	fmt.Fprintf(n.f, "\n")

	n.ex = NewExecutor(n.vars)
	for _, node := range n.nodes {
		n.processNode(node)
	}

	n.constructTrie(n.builds, nil, 0)

	for _, leaf := range n.leafs {
		fmt.Fprintf(n.f, "rule %s\n", leaf.name)
		fmt.Fprintf(n.f, " command = ")
		var nodes []*NinjaTrieNode
		for node := leaf; node != nil; node = node.parent {
			nodes = append(nodes, node)
		}
		for i := len(nodes) - 1; i >= 0; i-- {
			fmt.Fprintf(n.f, "%s", nodes[i].val)
		}
		fmt.Fprintf(n.f, "\n")
		fmt.Fprintf(n.f, " description = build $out\n")
	}

	for _, b := range n.builds {
		n.emitBuild(b.output, b.trie.name, b.deps)
		if b.depfile != "" {
			fmt.Fprintf(n.f, " depfile = %s\n", b.depfile)
		}
		for i, p := range b.params {
			fmt.Fprintf(n.f, " %s = %s\n", n.genParamName(i), p)
		}
	}

	n.f.Close()
}

func GenerateNinja(g *DepGraph) {
	NewNinjaGenerator(g).run()
}
