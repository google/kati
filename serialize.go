package main

import (
	"encoding/json"
	"os"
)

type SerializableDepNode struct {
	Output             string
	Cmds               []string
	Deps               []string
	HasRule            bool
	IsOrderOnly        bool
	IsPhony            bool
	ActualInputs       []string
	TargetSpecificVars Vars
	Filename           string
	Lineno             int
}

func MakeSerializable(nodes []*DepNode, done map[string]bool) (r []*SerializableDepNode) {
	for _, n := range nodes {
		if done[n.Output] {
			continue
		}
		done[n.Output] = true

		var deps []string
		for _, d := range n.Deps {
			deps = append(deps, d.Output)
		}
		r = append(r, &SerializableDepNode{
			Output:             n.Output,
			Cmds:               n.Cmds,
			Deps:               deps,
			HasRule:            n.HasRule,
			IsOrderOnly:        n.IsOrderOnly,
			IsPhony:            n.IsPhony,
			ActualInputs:       n.ActualInputs,
			TargetSpecificVars: n.TargetSpecificVars,
			Filename:           n.Filename,
			Lineno:             n.Lineno,
		})
		r = append(r, MakeSerializable(n.Deps, done)...)
	}
	return r
}

func DumpDepNodesAsJson(nodes []*DepNode, filename string) {
	n := MakeSerializable(nodes, make(map[string]bool))
	o, err := json.MarshalIndent(n, " ", " ")
	if err != nil {
		panic(err)
	}
	f, err2 := os.Create(filename)
	if err2 != nil {
		panic(err2)
	}
	f.Write(o)
}
