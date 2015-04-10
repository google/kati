package main

import (
	"bytes"
	"fmt"
	"io"
)

type Var interface {
	Value
	Append(*Evaluator, string) Var
	Flavor() string
	Origin() string
	IsDefined() bool
}

type SimpleVar struct {
	value  string
	origin string
}

func (v SimpleVar) Flavor() string  { return "simple" }
func (v SimpleVar) Origin() string  { return v.origin }
func (v SimpleVar) IsDefined() bool { return true }

func (v SimpleVar) String() string { return v.value }
func (v SimpleVar) Eval(w io.Writer, ev *Evaluator) {
	fmt.Fprint(w, v.value)
}

func (v SimpleVar) Append(ev *Evaluator, s string) Var {
	val, _, err := parseExpr([]byte(s), nil)
	if err != nil {
		panic(err)
	}
	var buf bytes.Buffer
	buf.WriteString(v.value)
	buf.WriteByte(' ')
	val.Eval(&buf, ev)
	v.value = buf.String()
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

func (v RecursiveVar) Append(_ *Evaluator, s string) Var {
	var buf bytes.Buffer
	buf.WriteString(v.expr.String())
	buf.WriteByte(' ')
	buf.WriteString(s)
	e, _, err := parseExpr(buf.Bytes(), nil)
	if err != nil {
		panic(err)
	}
	v.expr = e
	return v
}

type UndefinedVar struct{}

func (_ UndefinedVar) Flavor() string  { return "undefined" }
func (_ UndefinedVar) Origin() string  { return "undefined" }
func (_ UndefinedVar) IsDefined() bool { return false }
func (_ UndefinedVar) String() string  { return "" }
func (_ UndefinedVar) Eval(_ io.Writer, _ *Evaluator) {
}

func (_ UndefinedVar) Append(*Evaluator, string) Var {
	return UndefinedVar{}
}

type Vars map[string]Var

type VarTab struct {
	m      Vars
	parent *VarTab
}

func NewVarTab(vt *VarTab) *VarTab {
	return &VarTab{
		m:      make(map[string]Var),
		parent: vt,
	}
}

func (vt *VarTab) Vars() Vars {
	m := make(map[string]Var)
	if vt.parent != nil {
		for k, v := range vt.parent.Vars() {
			m[k] = v
		}
	}
	for k, v := range vt.m {
		m[k] = v
	}
	return m
}

func (vt Vars) Lookup(name string) Var {
	if v, ok := vt[name]; ok {
		return v
	}
	return UndefinedVar{}
}

func (vt *VarTab) Lookup(name string) Var {
	if v, ok := vt.m[name]; ok {
		return v
	}
	if vt.parent != nil {
		return vt.parent.Lookup(name)
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

func (vt *VarTab) Assign(name string, v Var) {
	vt.m.Assign(name, v)
}

func (vt *VarTab) Delete(name string) {
	delete(vt.m, name)
}
