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
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
)

const (
	valueTypeRecursive = 'R'
	valueTypeSimple    = 'S'
	valueTypeTSV       = 'T'
	valueTypeUndefined = 'U'
	valueTypeAssign    = 'a'
	valueTypeExpr      = 'e'
	valueTypeFunc      = 'f'
	valueTypeLiteral   = 'l'
	valueTypeNop       = 'n'
	valueTypeParamref  = 'p'
	valueTypeVarref    = 'r'
	valueTypeVarsubst  = 's'
	valueTypeTmpval    = 't'
)

// JSON is a json loader/saver.
var JSON LoadSaver

// GOB is a gob loader/saver.
var GOB LoadSaver

func init() {
	JSON = jsonLoadSaver{}
	GOB = gobLoadSaver{}
}

type jsonLoadSaver struct{}
type gobLoadSaver struct{}

type dumpbuf struct {
	w   bytes.Buffer
	err error
}

func (d *dumpbuf) Int(i int) {
	if d.err != nil {
		return
	}
	v := int32(i)
	d.err = binary.Write(&d.w, binary.LittleEndian, &v)
}

func (d *dumpbuf) Str(s string) {
	if d.err != nil {
		return
	}
	d.Int(len(s))
	if d.err != nil {
		return
	}
	_, d.err = io.WriteString(&d.w, s)
}

func (d *dumpbuf) Bytes(b []byte) {
	if d.err != nil {
		return
	}
	d.Int(len(b))
	if d.err != nil {
		return
	}
	_, d.err = d.w.Write(b)
}

func (d *dumpbuf) Byte(b byte) {
	if d.err != nil {
		return
	}
	d.err = writeByte(&d.w, b)
}

type serializableVar struct {
	Type     string
	V        string
	Origin   string
	Children []serializableVar
}

type serializableDepNode struct {
	Output             int
	Cmds               []string
	Deps               []int
	OrderOnlys         []int
	Parents            []int
	HasRule            bool
	IsPhony            bool
	ActualInputs       []int
	TargetSpecificVars []int
	Filename           string
	Lineno             int
}

type serializableTargetSpecificVar struct {
	Name  string
	Value serializableVar
}

type serializableGraph struct {
	Nodes       []*serializableDepNode
	Vars        map[string]serializableVar
	Tsvs        []serializableTargetSpecificVar
	Targets     []string
	Roots       []string
	AccessedMks []*accessedMakefile
	Exports     map[string]bool
}

func encGob(v interface{}) (string, error) {
	var buf bytes.Buffer
	e := gob.NewEncoder(&buf)
	err := e.Encode(v)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func encVar(k string, v Var) (string, error) {
	var dump dumpbuf
	dump.Str(k)
	v.dump(&dump)
	return dump.w.String(), dump.err
}

type depNodesSerializer struct {
	nodes     []*serializableDepNode
	tsvs      []serializableTargetSpecificVar
	tsvMap    map[string]int
	targets   []string
	targetMap map[string]int
	done      map[string]bool
	err       error
}

func newDepNodesSerializer() *depNodesSerializer {
	return &depNodesSerializer{
		tsvMap:    make(map[string]int),
		targetMap: make(map[string]int),
		done:      make(map[string]bool),
	}
}

func (ns *depNodesSerializer) serializeTarget(t string) int {
	id, present := ns.targetMap[t]
	if present {
		return id
	}
	id = len(ns.targets)
	ns.targetMap[t] = id
	ns.targets = append(ns.targets, t)
	return id
}

func (ns *depNodesSerializer) serializeDepNodes(nodes []*DepNode) {
	if ns.err != nil {
		return
	}
	for _, n := range nodes {
		if ns.done[n.Output] {
			continue
		}
		ns.done[n.Output] = true

		var deps []int
		for _, d := range n.Deps {
			deps = append(deps, ns.serializeTarget(d.Output))
		}
		var orderonlys []int
		for _, d := range n.OrderOnlys {
			orderonlys = append(orderonlys, ns.serializeTarget(d.Output))
		}
		var parents []int
		for _, d := range n.Parents {
			parents = append(parents, ns.serializeTarget(d.Output))
		}
		var actualInputs []int
		for _, i := range n.ActualInputs {
			actualInputs = append(actualInputs, ns.serializeTarget(i))
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
			sv := serializableTargetSpecificVar{Name: k, Value: v.serialize()}
			//gob := encGob(sv)
			gob, err := encVar(k, v)
			if err != nil {
				ns.err = err
				return
			}
			id, present := ns.tsvMap[gob]
			if !present {
				id = len(ns.tsvs)
				ns.tsvMap[gob] = id
				ns.tsvs = append(ns.tsvs, sv)
			}
			vars = append(vars, id)
		}

		ns.nodes = append(ns.nodes, &serializableDepNode{
			Output:             ns.serializeTarget(n.Output),
			Cmds:               n.Cmds,
			Deps:               deps,
			OrderOnlys:         orderonlys,
			Parents:            parents,
			HasRule:            n.HasRule,
			IsPhony:            n.IsPhony,
			ActualInputs:       actualInputs,
			TargetSpecificVars: vars,
			Filename:           n.Filename,
			Lineno:             n.Lineno,
		})
		ns.serializeDepNodes(n.Deps)
		if ns.err != nil {
			return
		}
		ns.serializeDepNodes(n.OrderOnlys)
		if ns.err != nil {
			return
		}
	}
}

func makeSerializableVars(vars Vars) (r map[string]serializableVar) {
	r = make(map[string]serializableVar)
	for k, v := range vars {
		r[k] = v.serialize()
	}
	return r
}

func makeSerializableGraph(g *DepGraph, roots []string) (serializableGraph, error) {
	ns := newDepNodesSerializer()
	ns.serializeDepNodes(g.nodes)
	v := makeSerializableVars(g.vars)
	return serializableGraph{
		Nodes:       ns.nodes,
		Vars:        v,
		Tsvs:        ns.tsvs,
		Targets:     ns.targets,
		Roots:       roots,
		AccessedMks: g.accessedMks,
		Exports:     g.exports,
	}, ns.err
}

func (jsonLoadSaver) Save(g *DepGraph, filename string, roots []string) error {
	startTime := time.Now()
	sg, err := makeSerializableGraph(g, roots)
	if err != nil {
		return err
	}
	o, err := json.MarshalIndent(sg, " ", " ")
	if err != nil {
		return err
	}
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	_, err = f.Write(o)
	if err != nil {
		f.Close()
		return err
	}
	err = f.Close()
	if err != nil {
		return err
	}
	logStats("json serialize time: %q", time.Since(startTime))
	return nil
}

func (gobLoadSaver) Save(g *DepGraph, filename string, roots []string) error {
	startTime := time.Now()
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	e := gob.NewEncoder(f)
	var sg serializableGraph
	{
		startTime := time.Now()
		sg, err = makeSerializableGraph(g, roots)
		if err != nil {
			return err
		}
		logStats("gob serialize prepare time: %q", time.Since(startTime))
	}
	{
		startTime := time.Now()
		err = e.Encode(sg)
		if err != nil {
			return err
		}
		logStats("gob serialize output time: %q", time.Since(startTime))
	}
	err = f.Close()
	if err != nil {
		return err
	}
	logStats("gob serialize time: %q", time.Since(startTime))
	return nil
}

func cacheFilename(mk string, roots []string) string {
	filename := ".kati_cache." + mk
	for _, r := range roots {
		filename += "." + r
	}
	return url.QueryEscape(filename)
}

func saveCache(g *DepGraph, roots []string) error {
	if len(g.accessedMks) == 0 {
		return fmt.Errorf("no Makefile is read")
	}
	cacheFile := cacheFilename(g.accessedMks[0].Filename, roots)
	for _, mk := range g.accessedMks {
		// Inconsistent, do not dump this result.
		if mk.State == fileInconsistent {
			if exists(cacheFile) {
				os.Remove(cacheFile)
			}
			return nil
		}
	}
	return GOB.Save(g, cacheFile, roots)
}

func deserializeSingleChild(sv serializableVar) (Value, error) {
	if len(sv.Children) != 1 {
		return nil, fmt.Errorf("unexpected number of children: %q", sv)
	}
	return deserializeVar(sv.Children[0])
}

func deserializeVar(sv serializableVar) (r Value, err error) {
	switch sv.Type {
	case "literal":
		return literal(sv.V), nil
	case "tmpval":
		return tmpval([]byte(sv.V)), nil
	case "expr":
		var e expr
		for _, v := range sv.Children {
			dv, err := deserializeVar(v)
			if err != nil {
				return nil, err
			}
			e = append(e, dv)
		}
		return e, nil
	case "varref":
		dv, err := deserializeSingleChild(sv)
		if err != nil {
			return nil, err
		}
		return &varref{varname: dv, paren: sv.V[0]}, nil
	case "paramref":
		v, err := strconv.Atoi(sv.V)
		if err != nil {
			return nil, err
		}
		return paramref(v), nil
	case "varsubst":
		varname, err := deserializeVar(sv.Children[0])
		if err != nil {
			return nil, err
		}
		pat, err := deserializeVar(sv.Children[1])
		if err != nil {
			return nil, err
		}
		subst, err := deserializeVar(sv.Children[2])
		if err != nil {
			return nil, err
		}
		return varsubst{
			varname: varname,
			pat:     pat,
			subst:   subst,
			paren:   sv.V[0],
		}, nil

	case "func":
		dv, err := deserializeVar(sv.Children[0])
		if err != nil {
			return nil, err
		}
		name, ok := dv.(literal)
		if !ok {
			return nil, fmt.Errorf("func name is not literal %s: %T", dv, dv)
		}
		f := funcMap[string(name[1:])]()
		f.AddArg(name)
		for _, a := range sv.Children[1:] {
			dv, err := deserializeVar(a)
			if err != nil {
				return nil, err
			}
			f.AddArg(dv)
		}
		return f, nil
	case "funcEvalAssign":
		rhs, err := deserializeVar(sv.Children[2])
		if err != nil {
			return nil, err
		}
		return &funcEvalAssign{
			lhs: sv.Children[0].V,
			op:  sv.Children[1].V,
			rhs: rhs,
		}, nil
	case "funcNop":
		return &funcNop{expr: sv.V}, nil

	case "simple":
		return &simpleVar{
			value:  strings.Split(sv.V, " "),
			origin: sv.Origin,
		}, nil
	case "recursive":
		expr, err := deserializeSingleChild(sv)
		if err != nil {
			return nil, err
		}
		return &recursiveVar{
			expr:   expr,
			origin: sv.Origin,
		}, nil

	case ":=", "=", "+=", "?=":
		dv, err := deserializeSingleChild(sv)
		if err != nil {
			return nil, err
		}
		v, ok := dv.(Var)
		if !ok {
			return nil, fmt.Errorf("not var: target specific var %s %T", dv, dv)
		}
		return &targetSpecificVar{
			v:  v,
			op: sv.Type,
		}, nil

	default:
		return nil, fmt.Errorf("unknown serialized variable type: %q", sv)
	}
}

func deserializeVars(vars map[string]serializableVar) (Vars, error) {
	r := make(Vars)
	for k, v := range vars {
		dv, err := deserializeVar(v)
		if err != nil {
			return nil, err
		}
		vv, ok := dv.(Var)
		if !ok {
			return nil, fmt.Errorf("not var: %s: %T", dv, dv)
		}
		r[k] = vv
	}
	return r, nil
}

func deserializeNodes(g serializableGraph) (r []*DepNode, err error) {
	nodes := g.Nodes
	tsvs := g.Tsvs
	targets := g.Targets
	// Deserialize all TSVs first so that multiple rules can share memory.
	var tsvValues []Var
	for _, sv := range tsvs {
		dv, err := deserializeVar(sv.Value)
		if err != nil {
			return nil, err
		}
		vv, ok := dv.(Var)
		if !ok {
			return nil, fmt.Errorf("not var: %s %T", dv, dv)
		}
		tsvValues = append(tsvValues, vv)
	}

	nodeMap := make(map[string]*DepNode)
	for _, n := range nodes {
		var actualInputs []string
		for _, i := range n.ActualInputs {
			actualInputs = append(actualInputs, targets[i])
		}

		d := &DepNode{
			Output:             targets[n.Output],
			Cmds:               n.Cmds,
			HasRule:            n.HasRule,
			IsPhony:            n.IsPhony,
			ActualInputs:       actualInputs,
			Filename:           n.Filename,
			Lineno:             n.Lineno,
			TargetSpecificVars: make(Vars),
		}

		for _, id := range n.TargetSpecificVars {
			sv := tsvs[id]
			d.TargetSpecificVars[sv.Name] = tsvValues[id]
		}

		nodeMap[targets[n.Output]] = d
		r = append(r, d)
	}

	for _, n := range nodes {
		d := nodeMap[targets[n.Output]]
		for _, o := range n.Deps {
			c, present := nodeMap[targets[o]]
			if !present {
				return nil, fmt.Errorf("unknown target: %d (%s)", o, targets[o])
			}
			d.Deps = append(d.Deps, c)
		}
		for _, o := range n.OrderOnlys {
			c, present := nodeMap[targets[o]]
			if !present {
				return nil, fmt.Errorf("unknown target: %d (%s)", o, targets[o])
			}
			d.OrderOnlys = append(d.OrderOnlys, c)
		}
		for _, o := range n.Parents {
			c, present := nodeMap[targets[o]]
			if !present {
				return nil, fmt.Errorf("unknown target: %d (%s)", o, targets[o])
			}
			d.Parents = append(d.Parents, c)
		}
	}

	return r, nil
}

func human(n int) string {
	if n >= 10*1000*1000*1000 {
		return fmt.Sprintf("%.2fGB", float32(n)/1000/1000/1000)
	}
	if n >= 10*1000*1000 {
		return fmt.Sprintf("%.2fMB", float32(n)/1000/1000)
	}
	if n >= 10*1000 {
		return fmt.Sprintf("%.2fkB", float32(n)/1000)
	}
	return fmt.Sprintf("%dB", n)
}

func showSerializedNodesStats(nodes []*serializableDepNode) {
	outputSize := 0
	cmdSize := 0
	depsSize := 0
	orderOnlysSize := 0
	actualInputSize := 0
	tsvSize := 0
	filenameSize := 0
	linenoSize := 0
	for _, n := range nodes {
		outputSize += 4
		for _, c := range n.Cmds {
			cmdSize += len(c)
		}
		depsSize += 4 * len(n.Deps)
		orderOnlysSize += 4 * len(n.OrderOnlys)
		actualInputSize += 4 * len(n.ActualInputs)
		tsvSize += 4 * len(n.TargetSpecificVars)
		filenameSize += len(n.Filename)
		linenoSize += 4
	}
	size := outputSize + cmdSize + depsSize + orderOnlysSize + actualInputSize + tsvSize + filenameSize + linenoSize
	logStats("%d nodes %s", len(nodes), human(size))
	logStats(" output %s", human(outputSize))
	logStats(" command %s", human(cmdSize))
	logStats(" deps %s", human(depsSize))
	logStats(" orderonlys %s", human(orderOnlysSize))
	logStats(" inputs %s", human(actualInputSize))
	logStats(" tsv %s", human(tsvSize))
	logStats(" filename %s", human(filenameSize))
	logStats(" lineno %s", human(linenoSize))
}

func (v serializableVar) size() int {
	size := 0
	size += len(v.Type)
	size += len(v.V)
	size += len(v.Origin)
	for _, c := range v.Children {
		size += c.size()
	}
	return size
}

func showSerializedVarsStats(vars map[string]serializableVar) {
	nameSize := 0
	valueSize := 0
	for k, v := range vars {
		nameSize += len(k)
		valueSize += v.size()
	}
	size := nameSize + valueSize
	logStats("%d vars %s", len(vars), human(size))
	logStats(" name %s", human(nameSize))
	logStats(" value %s", human(valueSize))
}

func showSerializedTsvsStats(vars []serializableTargetSpecificVar) {
	nameSize := 0
	valueSize := 0
	for _, v := range vars {
		nameSize += len(v.Name)
		valueSize += v.Value.size()
	}
	size := nameSize + valueSize
	logStats("%d tsvs %s", len(vars), human(size))
	logStats(" name %s", human(nameSize))
	logStats(" value %s", human(valueSize))
}

func showSerializedTargetsStats(targets []string) {
	size := 0
	for _, t := range targets {
		size += len(t)
	}
	logStats("%d targets %s", len(targets), human(size))
}

func showSerializedAccessedMksStats(accessedMks []*accessedMakefile) {
	size := 0
	for _, rm := range accessedMks {
		size += len(rm.Filename) + len(rm.Hash) + 4
	}
	logStats("%d makefiles %s", len(accessedMks), human(size))
}

func showSerializedGraphStats(g serializableGraph) {
	showSerializedNodesStats(g.Nodes)
	showSerializedVarsStats(g.Vars)
	showSerializedTsvsStats(g.Tsvs)
	showSerializedTargetsStats(g.Targets)
	showSerializedAccessedMksStats(g.AccessedMks)
}

func deserializeGraph(g serializableGraph) (*DepGraph, error) {
	if StatsFlag {
		showSerializedGraphStats(g)
	}
	nodes, err := deserializeNodes(g)
	if err != nil {
		return nil, err
	}
	vars, err := deserializeVars(g.Vars)
	if err != nil {
		return nil, err
	}
	return &DepGraph{
		nodes:       nodes,
		vars:        vars,
		accessedMks: g.AccessedMks,
		exports:     g.Exports,
	}, nil
}

func (jsonLoadSaver) Load(filename string) (*DepGraph, error) {
	startTime := time.Now()
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	d := json.NewDecoder(f)
	g := serializableGraph{Vars: make(map[string]serializableVar)}
	err = d.Decode(&g)
	if err != nil {
		return nil, err
	}
	dg, err := deserializeGraph(g)
	if err != nil {
		return nil, err
	}
	logStats("gob deserialize time: %q", time.Since(startTime))
	return dg, nil
}

func (gobLoadSaver) Load(filename string) (*DepGraph, error) {
	startTime := time.Now()
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	d := gob.NewDecoder(f)
	g := serializableGraph{Vars: make(map[string]serializableVar)}
	err = d.Decode(&g)
	if err != nil {
		return nil, err
	}
	dg, err := deserializeGraph(g)
	if err != nil {
		return nil, err
	}
	logStats("json deserialize time: %q", time.Since(startTime))
	return dg, nil
}

func loadCache(makefile string, roots []string) (*DepGraph, error) {
	startTime := time.Now()
	defer func() {
		logStats("Cache lookup time: %q", time.Since(startTime))
	}()

	filename := cacheFilename(makefile, roots)
	if !exists(filename) {
		glog.Warningf("Cache not found %q", filename)
		return nil, fmt.Errorf("cache not found: %s", filename)
	}

	g, err := GOB.Load(filename)
	if err != nil {
		glog.Warning("Cache load error %q: %v", filename, err)
		return nil, err
	}
	for _, mk := range g.accessedMks {
		if mk.State != fileExists && mk.State != fileNotExists {
			return nil, fmt.Errorf("internal error: broken state: %d", mk.State)
		}
		if mk.State == fileNotExists {
			if exists(mk.Filename) {
				glog.Infof("Cache expired: %s", mk.Filename)
				return nil, fmt.Errorf("cache expired: %s", mk.Filename)
			}
		} else {
			c, err := ioutil.ReadFile(mk.Filename)
			if err != nil {
				glog.Infof("Cache expired: %s", mk.Filename)
				return nil, fmt.Errorf("cache expired: %s", mk.Filename)
			}
			h := sha1.Sum(c)
			if !bytes.Equal(h[:], mk.Hash[:]) {
				glog.Infof("Cache expired: %s", mk.Filename)
				return nil, fmt.Errorf("cache expired: %s", mk.Filename)
			}
		}
	}
	glog.Info("Cache found in %q", filename)
	return g, nil
}
