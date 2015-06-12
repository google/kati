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

package main

import (
	"bytes"
	"io"
)

type Var interface {
	Value
	Append(*Evaluator, string) Var
	AppendVar(*Evaluator, Value) Var
	Flavor() string
	Origin() string
	IsDefined() bool
}

type TargetSpecificVar struct {
	v  Var
	op string
}

func (v TargetSpecificVar) Append(ev *Evaluator, s string) Var {
	return TargetSpecificVar{
		v:  v.v.Append(ev, s),
		op: v.op,
	}
}
func (v TargetSpecificVar) AppendVar(ev *Evaluator, v2 Value) Var {
	return TargetSpecificVar{
		v:  v.v.AppendVar(ev, v2),
		op: v.op,
	}
}
func (v TargetSpecificVar) Flavor() string {
	return v.v.Flavor()
}
func (v TargetSpecificVar) Origin() string {
	return v.v.Origin()
}
func (v TargetSpecificVar) IsDefined() bool {
	return v.v.IsDefined()
}
func (v TargetSpecificVar) String() string {
	// TODO: If we add the info of |op| a test starts
	// failing. Shouldn't we use this only for debugging?
	return v.v.String()
	// return v.v.String() + " (op=" + v.op + ")"
}
func (v TargetSpecificVar) Eval(w io.Writer, ev *Evaluator) {
	v.v.Eval(w, ev)
}

func (v TargetSpecificVar) Serialize() SerializableVar {
	return SerializableVar{
		Type:     v.op,
		Children: []SerializableVar{v.v.Serialize()},
	}
}

func (v TargetSpecificVar) Dump(w io.Writer) {
	dumpByte(w, ValueTypeTSV)
	dumpString(w, v.op)
	v.v.Dump(w)
}

type SimpleVar struct {
	// TODO(ukai): []byte -> Value (literal or so?)
	value  []byte
	origin string
}

func (v SimpleVar) Flavor() string  { return "simple" }
func (v SimpleVar) Origin() string  { return v.origin }
func (v SimpleVar) IsDefined() bool { return true }

func (v SimpleVar) String() string { return string(v.value) }
func (v SimpleVar) Eval(w io.Writer, ev *Evaluator) {
	w.Write(v.value)
}
func (v SimpleVar) Serialize() SerializableVar {
	return SerializableVar{
		Type:   "simple",
		V:      string(v.value),
		Origin: v.origin,
	}
}
func (v SimpleVar) Dump(w io.Writer) {
	dumpByte(w, ValueTypeSimple)
	dumpBytes(w, v.value)
	dumpString(w, v.origin)
}

func (v SimpleVar) Append(ev *Evaluator, s string) Var {
	val, _, err := parseExpr([]byte(s), nil, false)
	if err != nil {
		panic(err)
	}
	buf := bytes.NewBuffer(v.value)
	buf.WriteByte(' ')
	val.Eval(buf, ev)
	v.value = buf.Bytes()
	return v
}

func (v SimpleVar) AppendVar(ev *Evaluator, val Value) Var {
	buf := bytes.NewBuffer(v.value)
	buf.WriteByte(' ')
	val.Eval(buf, ev)
	v.value = buf.Bytes()
	return v
}

type RecursiveVar struct {
	expr   Value
	origin string
}

func (v RecursiveVar) Flavor() string  { return "recursive" }
func (v RecursiveVar) Origin() string  { return v.origin }
func (v RecursiveVar) IsDefined() bool { return true }

func (v RecursiveVar) String() string { return v.expr.String() }
func (v RecursiveVar) Eval(w io.Writer, ev *Evaluator) {
	v.expr.Eval(w, ev)
}
func (v RecursiveVar) Serialize() SerializableVar {
	return SerializableVar{
		Type:     "recursive",
		Children: []SerializableVar{v.expr.Serialize()},
		Origin:   v.origin,
	}
}
func (v RecursiveVar) Dump(w io.Writer) {
	dumpByte(w, ValueTypeRecursive)
	v.expr.Dump(w)
	dumpString(w, v.origin)
}

func (v RecursiveVar) Append(_ *Evaluator, s string) Var {
	var expr Expr
	if e, ok := v.expr.(Expr); ok {
		expr = append(e, literal(" "))
	} else {
		expr = Expr{v.expr, literal(" ")}
	}
	sv, _, err := parseExpr([]byte(s), nil, true)
	if err != nil {
		panic(err)
	}
	if aexpr, ok := sv.(Expr); ok {
		expr = append(expr, aexpr...)
	} else {
		expr = append(expr, sv)
	}
	v.expr = expr
	return v
}

func (v RecursiveVar) AppendVar(ev *Evaluator, val Value) Var {
	var buf bytes.Buffer
	buf.WriteString(v.expr.String())
	buf.WriteByte(' ')
	buf.WriteString(val.String())
	e, _, err := parseExpr(buf.Bytes(), nil, true)
	if err != nil {
		panic(err)
	}
	v.expr = e
	return v
}

type UndefinedVar struct{}

func (UndefinedVar) Flavor() string  { return "undefined" }
func (UndefinedVar) Origin() string  { return "undefined" }
func (UndefinedVar) IsDefined() bool { return false }
func (UndefinedVar) String() string  { return "" }
func (UndefinedVar) Eval(_ io.Writer, _ *Evaluator) {
}
func (UndefinedVar) Serialize() SerializableVar {
	return SerializableVar{Type: "undefined"}
}
func (UndefinedVar) Dump(w io.Writer) {
	dumpByte(w, ValueTypeUndefined)
}

func (UndefinedVar) Append(*Evaluator, string) Var {
	return UndefinedVar{}
}

func (UndefinedVar) AppendVar(_ *Evaluator, val Value) Var {
	return UndefinedVar{}
}

type Vars map[string]Var

func (vt Vars) Lookup(name string) Var {
	if v, ok := vt[name]; ok {
		return v
	}
	return UndefinedVar{}
}

func (vt Vars) Assign(name string, v Var) {
	switch v.Origin() {
	case "override", "environment override":
	default:
		ov := vt.Lookup(name)
		if ov.Origin() == "command line" {
			return
		}
	}
	vt[name] = v
}

func NewVars(vt Vars) Vars {
	r := make(Vars)
	r.Merge(vt)
	return r
}

func (vt Vars) Merge(vt2 Vars) {
	for k, v := range vt2 {
		vt[k] = v
	}
}

// save saves value of the variable named name.
// calling returned value will restore to the old value at the time
// when save called.
func (vt Vars) save(name string) func() {
	v := vt.Lookup(name)
	if v.IsDefined() {
		return func() {
			vt[name] = v
		}
	}
	return func() {
		delete(vt, name)
	}
}
