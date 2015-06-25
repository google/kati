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
	"fmt"
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

type targetSpecificVar struct {
	v  Var
	op string
}

func (v *targetSpecificVar) Append(ev *Evaluator, s string) Var {
	return &targetSpecificVar{
		v:  v.v.Append(ev, s),
		op: v.op,
	}
}
func (v *targetSpecificVar) AppendVar(ev *Evaluator, v2 Value) Var {
	return &targetSpecificVar{
		v:  v.v.AppendVar(ev, v2),
		op: v.op,
	}
}
func (v *targetSpecificVar) Flavor() string {
	return v.v.Flavor()
}
func (v *targetSpecificVar) Origin() string {
	return v.v.Origin()
}
func (v *targetSpecificVar) IsDefined() bool {
	return v.v.IsDefined()
}
func (v *targetSpecificVar) String() string {
	// TODO: If we add the info of |op| a test starts
	// failing. Shouldn't we use this only for debugging?
	return v.v.String()
	// return v.v.String() + " (op=" + v.op + ")"
}
func (v *targetSpecificVar) Eval(w io.Writer, ev *Evaluator) {
	v.v.Eval(w, ev)
}

func (v *targetSpecificVar) serialize() serializableVar {
	return serializableVar{
		Type:     v.op,
		Children: []serializableVar{v.v.serialize()},
	}
}

func (v *targetSpecificVar) dump(w io.Writer) {
	dumpByte(w, valueTypeTSV)
	dumpString(w, v.op)
	v.v.dump(w)
}

type simpleVar struct {
	value  string
	origin string
}

func (v *simpleVar) Flavor() string  { return "simple" }
func (v *simpleVar) Origin() string  { return v.origin }
func (v *simpleVar) IsDefined() bool { return true }

func (v *simpleVar) String() string { return v.value }
func (v *simpleVar) Eval(w io.Writer, ev *Evaluator) {
	io.WriteString(w, v.value)
}
func (v *simpleVar) serialize() serializableVar {
	return serializableVar{
		Type:   "simple",
		V:      v.value,
		Origin: v.origin,
	}
}
func (v *simpleVar) dump(w io.Writer) {
	dumpByte(w, valueTypeSimple)
	dumpString(w, v.value)
	dumpString(w, v.origin)
}

func (v *simpleVar) Append(ev *Evaluator, s string) Var {
	val, _, err := parseExpr([]byte(s), nil, false)
	if err != nil {
		panic(err)
	}
	abuf := newBuf()
	io.WriteString(abuf, v.value)
	writeByte(abuf, ' ')
	val.Eval(abuf, ev)
	v.value = abuf.String()
	freeBuf(abuf)
	return v
}

func (v *simpleVar) AppendVar(ev *Evaluator, val Value) Var {
	abuf := newBuf()
	io.WriteString(abuf, v.value)
	writeByte(abuf, ' ')
	val.Eval(abuf, ev)
	v.value = abuf.String()
	freeBuf(abuf)
	return v
}

type automaticVar struct {
	value []byte
}

func (v *automaticVar) Flavor() string  { return "simple" }
func (v *automaticVar) Origin() string  { return "automatic" }
func (v *automaticVar) IsDefined() bool { return true }

func (v *automaticVar) String() string { return string(v.value) }
func (v *automaticVar) Eval(w io.Writer, ev *Evaluator) {
	w.Write(v.value)
}
func (v *automaticVar) serialize() serializableVar {
	panic(fmt.Sprintf("cannnot serialize automatic var:%s", v.value))
}
func (v *automaticVar) dump(w io.Writer) {
	panic(fmt.Sprintf("cannnot dump automatic var:%s", v.value))
}

func (v *automaticVar) Append(ev *Evaluator, s string) Var {
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

func (v *automaticVar) AppendVar(ev *Evaluator, val Value) Var {
	buf := bytes.NewBuffer(v.value)
	buf.WriteByte(' ')
	val.Eval(buf, ev)
	v.value = buf.Bytes()
	return v
}

type recursiveVar struct {
	expr   Value
	origin string
}

func (v *recursiveVar) Flavor() string  { return "recursive" }
func (v *recursiveVar) Origin() string  { return v.origin }
func (v *recursiveVar) IsDefined() bool { return true }

func (v *recursiveVar) String() string { return v.expr.String() }
func (v *recursiveVar) Eval(w io.Writer, ev *Evaluator) {
	v.expr.Eval(w, ev)
}
func (v *recursiveVar) serialize() serializableVar {
	return serializableVar{
		Type:     "recursive",
		Children: []serializableVar{v.expr.serialize()},
		Origin:   v.origin,
	}
}
func (v *recursiveVar) dump(w io.Writer) {
	dumpByte(w, valueTypeRecursive)
	v.expr.dump(w)
	dumpString(w, v.origin)
}

func (v *recursiveVar) Append(_ *Evaluator, s string) Var {
	var exp expr
	if e, ok := v.expr.(expr); ok {
		exp = append(e, literal(" "))
	} else {
		exp = expr{v.expr, literal(" ")}
	}
	sv, _, err := parseExpr([]byte(s), nil, true)
	if err != nil {
		panic(err)
	}
	if aexpr, ok := sv.(expr); ok {
		exp = append(exp, aexpr...)
	} else {
		exp = append(exp, sv)
	}
	v.expr = exp
	return v
}

func (v *recursiveVar) AppendVar(ev *Evaluator, val Value) Var {
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

type undefinedVar struct{}

func (undefinedVar) Flavor() string  { return "undefined" }
func (undefinedVar) Origin() string  { return "undefined" }
func (undefinedVar) IsDefined() bool { return false }
func (undefinedVar) String() string  { return "" }
func (undefinedVar) Eval(_ io.Writer, _ *Evaluator) {
}
func (undefinedVar) serialize() serializableVar {
	return serializableVar{Type: "undefined"}
}
func (undefinedVar) dump(w io.Writer) {
	dumpByte(w, valueTypeUndefined)
}

func (undefinedVar) Append(*Evaluator, string) Var {
	return undefinedVar{}
}

func (undefinedVar) AppendVar(_ *Evaluator, val Value) Var {
	return undefinedVar{}
}

type Vars map[string]Var

func (vt Vars) Lookup(name string) Var {
	if v, ok := vt[name]; ok {
		return v
	}
	return undefinedVar{}
}

func (vt Vars) Assign(name string, v Var) {
	switch v.Origin() {
	case "override", "environment override":
	default:
		if ov, ok := vt[name]; ok {
			if ov.Origin() == "command line" {
				return
			}
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
	if v, ok := vt[name]; ok {
		return func() {
			vt[name] = v
		}
	}
	return func() {
		delete(vt, name)
	}
}
