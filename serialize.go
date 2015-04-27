package main

import (
	"encoding/gob"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
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
	TargetSpecificVars map[string]SerializableVar
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

		vars := make(map[string]SerializableVar)
		for k, v := range n.TargetSpecificVars {
			vars[k] = v.Serialize()
		}

		r = append(r, &SerializableDepNode{
			Output:             n.Output,
			Cmds:               n.Cmds,
			Deps:               deps,
			HasRule:            n.HasRule,
			IsOrderOnly:        n.IsOrderOnly,
			IsPhony:            n.IsPhony,
			ActualInputs:       n.ActualInputs,
			TargetSpecificVars: vars,
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

func DumpDepGraph(nodes []*DepNode, vars Vars, filename string) {
	n := MakeSerializableDepNodes(nodes, make(map[string]bool))
	v := MakeSerializableVars(vars)

	f, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	e := gob.NewEncoder(f)
	e.Encode(SerializableGraph{Nodes: n, Vars: v})
}

func DeserializeSingleChild(sv SerializableVar) Value {
	if len(sv.Children) != 1 {
		panic(fmt.Sprintf("unexpected number of children: %q", sv))
	}
	return DeserializeVar(sv.Children[0])
}

func DeserializeVar(sv SerializableVar) (r Value) {
	switch sv.Type {
	case "literal":
		return literal(sv.V)
	case "expr":
		var e Expr
		for _, v := range sv.Children {
			e = append(e, DeserializeVar(v))
		}
		return e
	case "varref":
		return varref{varname: DeserializeSingleChild(sv) }
	case "paramref":
		v, err := strconv.Atoi(sv.V)
		if err != nil {
			panic(err)
		}
		return paramref(v)

	case "func":
		name := DeserializeVar(sv.Children[0]).(literal)
		f := funcMap[string(name[1:])]()
		f.AddArg(name)
		for _, a := range sv.Children[1:] {
			f.AddArg(DeserializeVar(a))
		}
		return f
	case "funcEvalAssign":
		return &funcEvalAssign{
			lhs: sv.Children[0].V,
			op: sv.Children[1].V,
			rhs: DeserializeVar(sv.Children[2]),
		}
	case "funcNop":
		return &funcNop{ expr: sv.V }

	case "simple":
		return SimpleVar{
			value: []byte(sv.V),
			origin: sv.Origin,
		}
	case "recursive":
		return RecursiveVar{
			expr: DeserializeSingleChild(sv),
			origin: sv.Origin,
		}

	case ":=", "=", "+=", "?=":
		return TargetSpecificVar{
			v: DeserializeSingleChild(sv).(Var),
			op: sv.Type,
		}

	default:
		panic(fmt.Sprintf("unknown serialized variable type: %q", sv))
	}
	return UndefinedVar{}
}

func DeserializeVars(vars map[string]SerializableVar) (Vars) {
	r := make(Vars)
	for k, v := range vars {
		r[k] = DeserializeVar(v).(Var)
	}
	return r
}

func DeserializeNodes(nodes []*SerializableDepNode) (r []*DepNode) {
	nodeMap := make(map[string]*DepNode)
	for _, n := range nodes {
		d := &DepNode{
			Output: n.Output,
			Cmds: n.Cmds,
			HasRule: n.HasRule,
			IsOrderOnly: n.IsOrderOnly,
			IsPhony: n.IsPhony,
			ActualInputs: n.ActualInputs,
			Filename: n.Filename,
			Lineno: n.Lineno,
			TargetSpecificVars: make(Vars),
		}

		for k, v := range n.TargetSpecificVars {
			d.TargetSpecificVars[k] = DeserializeVar(v).(Var)
		}

		nodeMap[n.Output] = d
		r = append(r, d)
	}

	for _, n := range nodes {
		d := nodeMap[n.Output]
		for _, o := range n.Deps {
			c, present := nodeMap[o]
			if !present {
				panic(fmt.Sprintf("unknown target: %s", o))
			}
			d.Deps = append(d.Deps, c)
		}
	}

	return r
}

func LoadDepGraphFromJson(filename string) ([]*DepNode, Vars) {
	f, err := os.Open(filename)
	if err != nil {
		panic(err)
	}

	d := json.NewDecoder(f)
	g := SerializableGraph{ Vars: make(map[string]SerializableVar) }
	err = d.Decode(&g)
	if err != nil {
		panic(err)
	}

	nodes := DeserializeNodes(g.Nodes)
	vars := DeserializeVars(g.Vars)
	return nodes, vars
}

func LoadDepGraph(filename string) ([]*DepNode, Vars) {
	f, err := os.Open(filename)
	if err != nil {
		panic(err)
	}

	d := gob.NewDecoder(f)
	g := SerializableGraph{ Vars: make(map[string]SerializableVar) }
	err = d.Decode(&g)
	if err != nil {
		panic(err)
	}

	nodes := DeserializeNodes(g.Nodes)
	vars := DeserializeVars(g.Vars)
	return nodes, vars
}
