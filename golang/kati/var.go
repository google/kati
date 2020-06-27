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
	"strings"
)

// Var is an interface of make variable.
type Var interface {
	Value
	Append(*Evaluator, string) (Var, error)
	AppendVar(*Evaluator, Value) (Var, error)
	Flavor() string
	Origin() string
	IsDefined() bool
}

type targetSpecificVar struct {
	v  Var
	op string
}

func (v *targetSpecificVar) Append(ev *Evaluator, s string) (Var, error) {
	nv, err := v.v.Append(ev, s)
	if err != nil {
		return nil, err
	}
	return &targetSpecificVar{
		v:  nv,
		op: v.op,
	}, nil
}
func (v *targetSpecificVar) AppendVar(ev *Evaluator, v2 Value) (Var, error) {
	nv, err := v.v.AppendVar(ev, v2)
	if err != nil {
		return nil, err
	}
	return &targetSpecificVar{
		v:  nv,
		op: v.op,
	}, nil
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
func (v *targetSpecificVar) Eval(w evalWriter, ev *Evaluator) error {
	return v.v.Eval(w, ev)
}

func (v *targetSpecificVar) serialize() serializableVar {
	return serializableVar{
		Type:     v.op,
		Children: []serializableVar{v.v.serialize()},
	}
}

func (v *targetSpecificVar) dump(d *dumpbuf) {
	d.Byte(valueTypeTSV)
	d.Str(v.op)
	v.v.dump(d)
}

type simpleVar struct {
	// space separated. note that each string may contain spaces, so
	// it is not word list.
	value  []string
	origin string
}

func (v *simpleVar) Flavor() string  { return "simple" }
func (v *simpleVar) Origin() string  { return v.origin }
func (v *simpleVar) IsDefined() bool { return true }

func (v *simpleVar) String() string { return strings.Join(v.value, " ") }
func (v *simpleVar) Eval(w evalWriter, ev *Evaluator) error {
	space := false
	for _, v := range v.value {
		if space {
			writeByte(w, ' ')
		}
		io.WriteString(w, v)
		space = true
	}
	return nil
}
func (v *simpleVar) serialize() serializableVar {
	return serializableVar{
		Type:   "simple",
		V:      v.String(),
		Origin: v.origin,
	}
}
func (v *simpleVar) dump(d *dumpbuf) {
	d.Byte(valueTypeSimple)
	d.Int(len(v.value))
	for _, v := range v.value {
		d.Str(v)
	}
	d.Str(v.origin)
}

func (v *simpleVar) Append(ev *Evaluator, s string) (Var, error) {
	val, _, err := parseExpr([]byte(s), nil, parseOp{})
	if err != nil {
		return nil, err
	}
	abuf := newEbuf()
	err = val.Eval(abuf, ev)
	if err != nil {
		return nil, err
	}
	v.value = append(v.value, abuf.String())
	abuf.release()
	return v, nil
}

func (v *simpleVar) AppendVar(ev *Evaluator, val Value) (Var, error) {
	abuf := newEbuf()
	err := val.Eval(abuf, ev)
	if err != nil {
		return nil, err
	}
	v.value = append(v.value, abuf.String())
	abuf.release()
	return v, nil
}

type automaticVar struct {
	value []byte
}

func (v *automaticVar) Flavor() string  { return "simple" }
func (v *automaticVar) Origin() string  { return "automatic" }
func (v *automaticVar) IsDefined() bool { return true }

func (v *automaticVar) String() string { return string(v.value) }
func (v *automaticVar) Eval(w evalWriter, ev *Evaluator) error {
	w.Write(v.value)
	return nil
}
func (v *automaticVar) serialize() serializableVar {
	return serializableVar{Type: ""}
}
func (v *automaticVar) dump(d *dumpbuf) {
	d.err = fmt.Errorf("cannnot dump automatic var:%s", v.value)
}

func (v *automaticVar) Append(ev *Evaluator, s string) (Var, error) {
	val, _, err := parseExpr([]byte(s), nil, parseOp{})
	if err != nil {
		return nil, err
	}
	abuf := newEbuf()
	err = val.Eval(abuf, ev)
	if err != nil {
		return nil, err
	}
	value := []string{string(v.value), abuf.String()}
	abuf.release()
	return &simpleVar{
		value:  value,
		origin: "file",
	}, nil
}

func (v *automaticVar) AppendVar(ev *Evaluator, val Value) (Var, error) {
	abuf := newEbuf()
	err := val.Eval(abuf, ev)
	if err != nil {
		return nil, err
	}
	value := []string{string(v.value), abuf.String()}
	abuf.release()
	return &simpleVar{
		value:  value,
		origin: "file",
	}, nil
}

type recursiveVar struct {
	expr   Value
	origin string
}

func (v *recursiveVar) Flavor() string  { return "recursive" }
func (v *recursiveVar) Origin() string  { return v.origin }
func (v *recursiveVar) IsDefined() bool { return true }

func (v *recursiveVar) String() string { return v.expr.String() }
func (v *recursiveVar) Eval(w evalWriter, ev *Evaluator) error {
	v.expr.Eval(w, ev)
	return nil
}
func (v *recursiveVar) serialize() serializableVar {
	return serializableVar{
		Type:     "recursive",
		Children: []serializableVar{v.expr.serialize()},
		Origin:   v.origin,
	}
}
func (v *recursiveVar) dump(d *dumpbuf) {
	d.Byte(valueTypeRecursive)
	v.expr.dump(d)
	d.Str(v.origin)
}

func (v *recursiveVar) Append(_ *Evaluator, s string) (Var, error) {
	var exp expr
	if e, ok := v.expr.(expr); ok {
		exp = append(e, literal(" "))
	} else {
		exp = expr{v.expr, literal(" ")}
	}
	sv, _, err := parseExpr([]byte(s), nil, parseOp{alloc: true})
	if err != nil {
		return nil, err
	}
	if aexpr, ok := sv.(expr); ok {
		exp = append(exp, aexpr...)
	} else {
		exp = append(exp, sv)
	}
	v.expr = exp
	return v, nil
}

func (v *recursiveVar) AppendVar(ev *Evaluator, val Value) (Var, error) {
	var buf bytes.Buffer
	buf.WriteString(v.expr.String())
	buf.WriteByte(' ')
	buf.WriteString(val.String())
	e, _, err := parseExpr(buf.Bytes(), nil, parseOp{alloc: true})
	if err != nil {
		return nil, err
	}
	v.expr = e
	return v, nil
}

type undefinedVar struct{}

func (undefinedVar) Flavor() string  { return "undefined" }
func (undefinedVar) Origin() string  { return "undefined" }
func (undefinedVar) IsDefined() bool { return false }
func (undefinedVar) String() string  { return "" }
func (undefinedVar) Eval(_ evalWriter, _ *Evaluator) error {
	return nil
}
func (undefinedVar) serialize() serializableVar {
	return serializableVar{Type: "undefined"}
}
func (undefinedVar) dump(d *dumpbuf) {
	d.Byte(valueTypeUndefined)
}

func (undefinedVar) Append(*Evaluator, string) (Var, error) {
	return undefinedVar{}, nil
}

func (undefinedVar) AppendVar(_ *Evaluator, val Value) (Var, error) {
	return undefinedVar{}, nil
}

// Vars is a map for make variables.
type Vars map[string]Var

// usedEnvs tracks what environment variables are used.
var usedEnvs = map[string]bool{}

// Lookup looks up named make variable.
func (vt Vars) Lookup(name string) Var {
	if v, ok := vt[name]; ok {
		if strings.HasPrefix(v.Origin(), "environment") {
			usedEnvs[name] = true
		}
		return v
	}
	return undefinedVar{}
}

// origin precedence
//  override / environment override
//  command line
//  file
//  environment
//  default
// TODO(ukai): is this correct order?
var originPrecedence = map[string]int{
	"override":             4,
	"environment override": 4,
	"command line":         3,
	"file":                 2,
	"environment":          2,
	"default":              1,
	"undefined":            0,
	"automatic":            0,
}

// Assign assigns v to name.
func (vt Vars) Assign(name string, v Var) {
	vo := v.Origin()
	// assign automatic always win.
	// assign new value to automatic always win.
	if vo != "automatic" {
		vp := originPrecedence[v.Origin()]
		var op int
		if ov, ok := vt[name]; ok {
			op = originPrecedence[ov.Origin()]
		}
		if op > vp {
			return
		}
	}
	vt[name] = v
}

// NewVars creates new Vars.
func NewVars(vt Vars) Vars {
	r := make(Vars)
	r.Merge(vt)
	return r
}

// Merge merges vt2 into vt.
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
