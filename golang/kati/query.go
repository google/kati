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
	"fmt"
	"io"
)

func showDeps(w io.Writer, n *DepNode, indent int, seen map[string]int) {
	id, present := seen[n.Output]
	if !present {
		id = len(seen)
		seen[n.Output] = id
	}
	fmt.Fprintf(w, "%*c%s (%d)\n", indent, ' ', n.Output, id)
	if present {
		return
	}
	for _, d := range n.Deps {
		showDeps(w, d, indent+1, seen)
	}
	if len(n.OrderOnlys) > 0 {
		fmt.Fprintf(w, "%*corder_onlys:\n", indent, ' ')
		for _, d := range n.OrderOnlys {
			showDeps(w, d, indent+1, seen)
		}
	}
}

func showNode(w io.Writer, n *DepNode) {
	fmt.Fprintf(w, "%s:", n.Output)
	for _, i := range n.ActualInputs {
		fmt.Fprintf(w, " %s", i)
	}
	fmt.Fprintf(w, "\n")
	for _, c := range n.Cmds {
		fmt.Fprintf(w, "\t%s\n", c)
	}
	for k, v := range n.TargetSpecificVars {
		fmt.Fprintf(w, "%s: %s=%s\n", n.Output, k, v.String())
	}

	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "location: %s:%d\n", n.Filename, n.Lineno)
	if n.IsPhony {
		fmt.Fprintf(w, "phony: true\n")
	}

	seen := make(map[string]int)
	fmt.Fprintf(w, "dependencies:\n")
	showDeps(w, n, 1, seen)
}

func handleNodeQuery(w io.Writer, q string, nodes []*DepNode) {
	for _, n := range nodes {
		if n.Output == q {
			showNode(w, n)
			break
		}
	}
}

// Query queries q in g.
func Query(w io.Writer, q string, g *DepGraph) {
	if q == "$MAKEFILE_LIST" {
		for _, mk := range g.accessedMks {
			fmt.Fprintf(w, "%s: state=%d\n", mk.Filename, mk.State)
		}
		return
	}

	if q == "$*" {
		for k, v := range g.vars {
			fmt.Fprintf(w, "%s=%s\n", k, v.String())
		}
		return
	}

	if q == "*" {
		for _, n := range g.nodes {
			fmt.Fprintf(w, "%s\n", n.Output)
		}
		return
	}
	handleNodeQuery(w, q, g.nodes)
}
