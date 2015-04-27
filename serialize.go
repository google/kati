package main

import (
	"encoding/json"
	"os"
)

type SerializableVar struct {
	Type     string
	V        string
	Origin   string
	Children []SerializableVar
}

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

type SerializableGraph struct {
	Nodes []*SerializableDepNode
	Vars  map[string]SerializableVar
}

func MakeSerializableDepNodes(nodes []*DepNode, done map[string]bool) (r []*SerializableDepNode) {
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
		r = append(r, MakeSerializableDepNodes(n.Deps, done)...)
	}
	return r
}

func MakeSerializableVars(vars Vars) (r map[string]SerializableVar) {
	r = make(map[string]SerializableVar)
	for k, v := range vars {
		r[k] = v.Serialize()
	}
	return r
}

func DumpDepGraphAsJson(nodes []*DepNode, vars Vars, filename string) {
	n := MakeSerializableDepNodes(nodes, make(map[string]bool))
	v := MakeSerializableVars(vars)

	o, err := json.MarshalIndent(SerializableGraph{Nodes: n, Vars: v}, " ", " ")
	if err != nil {
		panic(err)
	}
	f, err2 := os.Create(filename)
	if err2 != nil {
		panic(err2)
	}
	f.Write(o)
}
