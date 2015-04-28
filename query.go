package main

import "fmt"

func showDeps(n *DepNode, indent int, seen map[string]int) {
	id, present := seen[n.Output]
	if !present {
		id = len(seen)
		seen[n.Output] = id
	}
	fmt.Printf("%*c%s (%d)\n", indent, ' ', n.Output, id)
	if present {
		return
	}
	for _, d := range n.Deps {
		showDeps(d, indent + 1, seen)
	}
}

func showNode(n *DepNode) {
	fmt.Printf("%s:", n.Output)
	for _, i := range n.ActualInputs {
		fmt.Printf(" %s", i)
	}
	fmt.Printf("\n")
	for _, c := range n.Cmds {
		fmt.Printf("\t%s\n", c)
	}
	for k, v := range n.TargetSpecificVars {
		fmt.Printf("%s: %s=%s\n", n.Output, k, v.String())
	}

	fmt.Printf("\n")
	fmt.Printf("location: %s:%d\n", n.Filename, n.Lineno)
	if n.IsOrderOnly {
		fmt.Printf("order-only: true\n")
	}
	if n.IsPhony {
		fmt.Printf("phony: true\n")
	}

	seen := make(map[string]int)
	fmt.Printf("dependencies:\n")
	showDeps(n, 1, seen)
}

func HandleNodeQuery(q string, nodes []*DepNode) {
	for _, n := range nodes {
		if n.Output == q {
			showNode(n)
			break
		}
	}
}

func HandleQuery(q string, nodes []*DepNode, vars Vars) {
	if q == "$*" {
		for k, v := range vars {
			fmt.Printf("%s=%s\n", k, v.String())
		}
		return
	}

	if q == "*" {
		for _, n := range nodes {
			fmt.Printf("%s\n", n.Output)
		}
		return
	}
	HandleNodeQuery(q, nodes)
}

