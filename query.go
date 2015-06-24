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
		showDeps(d, indent+1, seen)
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

func HandleQuery(q string, g *DepGraph) {
	if q == "$MAKEFILE_LIST" {
		for _, mk := range g.readMks {
			fmt.Printf("%s: state=%d\n", mk.Filename, mk.State)
		}
		return
	}

	if q == "$*" {
		for k, v := range g.vars {
			fmt.Printf("%s=%s\n", k, v.String())
		}
		return
	}

	if q == "*" {
		for _, n := range g.nodes {
			fmt.Printf("%s\n", n.Output)
		}
		return
	}
	HandleNodeQuery(q, g.nodes)
}
