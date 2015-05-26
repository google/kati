package main

import (
	"bytes"
	"fmt"
	"os"
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
		cmd := strings.Replace(r.cmd, "$", "$$", -1)
		cmd = strings.Replace(cmd, "\t", " ", -1)
		cmd = strings.Replace(cmd, "\\\n", " ", -1)
		buf.WriteString(cmd)
		if i == len(runners)-1 && r.ignoreError {
			buf.WriteString(" ; true")
		}
	}
	return buf.String()
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
		ruleName = fmt.Sprintf("rule%d", n.ruleId)
		n.ruleId++
		fmt.Fprintf(n.f, `rule %s
 command = %s
 description = build $out
`, ruleName, genShellScript(runners))
	}

	fmt.Fprintf(n.f, "build %s: %s", node.Output, ruleName)
	var deps []string
	var orderOnlys []string
	for _, d := range node.Deps {
		if d.IsOrderOnly {
			orderOnlys = append(orderOnlys, d.Output)
		} else {
			deps = append(deps, d.Output)
		}
	}
	if len(deps) > 0 {
		fmt.Fprintf(n.f, " %s", strings.Join(deps, " "))
	}
	if len(orderOnlys) > 0 {
		fmt.Fprintf(n.f, " | %s", strings.Join(orderOnlys, " "))
	}
	fmt.Fprintf(n.f, "\n")

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
