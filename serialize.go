package main

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"os"
	"sort"
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
	TargetSpecificVars []int
	Filename           string
	Lineno             int
}

type SerializableTargetSpecificVar struct {
	Name  string
	Value SerializableVar
}

type SerializableGraph struct {
	Nodes []*SerializableDepNode
	Vars  map[string]SerializableVar
	Tsvs  []SerializableTargetSpecificVar
}

func encGob(v interface{}) string {
	var buf bytes.Buffer
	e := gob.NewEncoder(&buf)
	err := e.Encode(v)
	if err != nil {
		panic(err)
	}
	return buf.String()
}

type DepNodesSerializer struct {
	nodes  []*SerializableDepNode
	tsvs   []SerializableTargetSpecificVar
	tsvMap map[string]int
	done   map[string]bool
}

func NewDepNodesSerializer() *DepNodesSerializer {
	return &DepNodesSerializer{
		tsvMap: make(map[string]int),
		done:   make(map[string]bool),
	}
}

func (ns *DepNodesSerializer) SerializeDepNodes(nodes []*DepNode) {
	for _, n := range nodes {
		if ns.done[n.Output] {
			continue
		}
		ns.done[n.Output] = true

		var deps []string
		for _, d := range n.Deps {
			deps = append(deps, d.Output)
		}

		// Sort keys for consistent serialization.
		var tsvKeys []string
		for k := range n.TargetSpecificVars {
			tsvKeys = append(tsvKeys, k)
		}
		sort.Strings(tsvKeys)

		var vars []int
		for _, k := range tsvKeys {
			v := n.TargetSpecificVars[k]
			sv := SerializableTargetSpecificVar{Name: k, Value: v.Serialize()}
			gob := encGob(sv)
			id, present := ns.tsvMap[gob]
			if !present {
				id = len(ns.tsvs)
				ns.tsvMap[gob] = id
				ns.tsvs = append(ns.tsvs, sv)
			}
			vars = append(vars, id)
		}

		ns.nodes = append(ns.nodes, &SerializableDepNode{
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
		ns.SerializeDepNodes(n.Deps)
	}
}

func MakeSerializableVars(vars Vars) (r map[string]SerializableVar) {
	r = make(map[string]SerializableVar)
	for k, v := range vars {
		r[k] = v.Serialize()
	}
	return r
}

func MakeSerializableGraph(nodes []*DepNode, vars Vars) SerializableGraph {
	ns := NewDepNodesSerializer()
	ns.SerializeDepNodes(nodes)
	v := MakeSerializableVars(vars)
	return SerializableGraph{Nodes: ns.nodes, Vars: v, Tsvs: ns.tsvs}
}

func DumpDepGraphAsJson(nodes []*DepNode, vars Vars, filename string) {
	o, err := json.MarshalIndent(MakeSerializableGraph(nodes, vars), " ", " ")
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
	f, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	e := gob.NewEncoder(f)
	e.Encode(MakeSerializableGraph(nodes, vars))
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
	case "tmpval":
		return tmpval([]byte(sv.V))
	case "expr":
		var e Expr
		for _, v := range sv.Children {
			e = append(e, DeserializeVar(v))
		}
		return e
	case "varref":
		return varref{varname: DeserializeSingleChild(sv)}
	case "paramref":
		v, err := strconv.Atoi(sv.V)
		if err != nil {
			panic(err)
		}
		return paramref(v)
	case "varsubst":
		return varsubst{
			varname: DeserializeVar(sv.Children[0]),
			pat:     DeserializeVar(sv.Children[1]),
			subst:   DeserializeVar(sv.Children[2]),
		}

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
			op:  sv.Children[1].V,
			rhs: DeserializeVar(sv.Children[2]),
		}
	case "funcNop":
		return &funcNop{expr: sv.V}

	case "simple":
		return SimpleVar{
			value:  []byte(sv.V),
			origin: sv.Origin,
		}
	case "recursive":
		return RecursiveVar{
			expr:   DeserializeSingleChild(sv),
			origin: sv.Origin,
		}

	case ":=", "=", "+=", "?=":
		return TargetSpecificVar{
			v:  DeserializeSingleChild(sv).(Var),
			op: sv.Type,
		}

	default:
		panic(fmt.Sprintf("unknown serialized variable type: %q", sv))
	}
	return UndefinedVar{}
}

func DeserializeVars(vars map[string]SerializableVar) Vars {
	r := make(Vars)
	for k, v := range vars {
		r[k] = DeserializeVar(v).(Var)
	}
	return r
}

func DeserializeNodes(nodes []*SerializableDepNode, tsvs []SerializableTargetSpecificVar) (r []*DepNode) {
	// Deserialize all TSVs first so that multiple rules can share memory.
	var tsvValues []Var
	for _, sv := range tsvs {
		tsvValues = append(tsvValues, DeserializeVar(sv.Value).(Var))
	}

	nodeMap := make(map[string]*DepNode)
	for _, n := range nodes {
		d := &DepNode{
			Output:             n.Output,
			Cmds:               n.Cmds,
			HasRule:            n.HasRule,
			IsOrderOnly:        n.IsOrderOnly,
			IsPhony:            n.IsPhony,
			ActualInputs:       n.ActualInputs,
			Filename:           n.Filename,
			Lineno:             n.Lineno,
			TargetSpecificVars: make(Vars),
		}

		for _, id := range n.TargetSpecificVars {
			sv := tsvs[id]
			d.TargetSpecificVars[sv.Name] = tsvValues[id]
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

func DeserializeGraph(g SerializableGraph) ([]*DepNode, Vars) {
	nodes := DeserializeNodes(g.Nodes, g.Tsvs)
	vars := DeserializeVars(g.Vars)
	return nodes, vars
}

func LoadDepGraphFromJson(filename string) ([]*DepNode, Vars) {
	f, err := os.Open(filename)
	if err != nil {
		panic(err)
	}

	d := json.NewDecoder(f)
	g := SerializableGraph{Vars: make(map[string]SerializableVar)}
	err = d.Decode(&g)
	if err != nil {
		panic(err)
	}
	return DeserializeGraph(g)
}

func LoadDepGraph(filename string) ([]*DepNode, Vars) {
	f, err := os.Open(filename)
	if err != nil {
		panic(err)
	}

	d := gob.NewDecoder(f)
	g := SerializableGraph{Vars: make(map[string]SerializableVar)}
	err = d.Decode(&g)
	if err != nil {
		panic(err)
	}
	return DeserializeGraph(g)
}
