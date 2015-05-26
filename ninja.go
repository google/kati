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

func genShellScript(r runner) string {
	var buf bytes.Buffer
	cmd := trimLeftSpace(r.cmd)
	cmd = strings.TrimRight(cmd, " \t\n;")
	cmd = strings.Replace(cmd, "$", "$$", -1)
	cmd = strings.Replace(cmd, "\t", " ", -1)
	cmd = strings.Replace(cmd, "\\\n", " ", -1)
	buf.WriteString(cmd)
	if r.ignoreError {
		buf.WriteString(" ; true")
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

func genIntermediateTargetName(o string, i int) string {
	return fmt.Sprintf(".make_targets/%s@%d", o, i)
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
	if len(runners) == 0 {
		n.emitBuild(node.Output, "phony", getDepString(node))
	} else {
		for i, r := range runners {
			cmd := genShellScript(r)
			output := node.Output
			if i < len(runners)-1 {
				output = genIntermediateTargetName(node.Output, i)
				cmd += " && touch $out"
			}

			ruleName := n.genRuleName()
			fmt.Fprintf(n.f, `rule %s
 command = %s
 description = build $out
`, ruleName, cmd)

			var dep string
			if i == 0 {
				dep = getDepString(node)
			} else {
				dep = genIntermediateTargetName(node.Output, i-1)
			}
			n.emitBuild(output, ruleName, dep)
		}
	}

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
