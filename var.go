package main

type Var interface {
	Value
	Flavor() string
	Origin() string
	IsDefined() bool
}

type Value interface {
	String() string
	Eval(ev *Evaluator) string
}

type SimpleVar struct {
	value  string
	origin string
}

func (v SimpleVar) Flavor() string  { return "simple" }
func (v SimpleVar) Origin() string  { return v.origin }
func (v SimpleVar) IsDefined() bool { return true }

func (v SimpleVar) String() string            { return v.value }
func (v SimpleVar) Eval(ev *Evaluator) string { return v.value }

type RecursiveVar struct {
	expr   string
	origin string
}

func (v RecursiveVar) Flavor() string  { return "recursive" }
func (v RecursiveVar) Origin() string  { return v.origin }
func (v RecursiveVar) IsDefined() bool { return true }
func (v RecursiveVar) String() string  { return v.expr }
func (v RecursiveVar) Eval(ev *Evaluator) string {
	return ev.evalExpr(v.expr)
}

type UndefinedVar struct{}

func (_ UndefinedVar) Flavor() string            { return "undefined" }
func (_ UndefinedVar) Origin() string            { return "" }
func (_ UndefinedVar) IsDefined() bool           { return false }
func (_ UndefinedVar) String() string            { return "" }
func (_ UndefinedVar) Eval(ev *Evaluator) string { return "" }

type VarTab struct {
	m      map[string]Var
	parent *VarTab
}

func NewVarTab(vt *VarTab) *VarTab {
	return &VarTab{
		m:      make(map[string]Var),
		parent: vt,
	}
}

func (vt *VarTab) Vars() map[string]Var {
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

func (vt *VarTab) Lookup(name string) Var {
	if v, ok := vt.m[name]; ok {
		return v
	}
	if vt.parent != nil {
		return vt.parent.Lookup(name)
	}
	return UndefinedVar{}
}

func (vt *VarTab) Assign(name string, v Var) {
	vt.m[name] = v
}
