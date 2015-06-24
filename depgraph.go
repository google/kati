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
	"crypto/sha1"
	"io/ioutil"
	"time"
)

type DepGraph struct {
	nodes    []*DepNode
	vars     Vars
	readMks  []*ReadMakefile
	exports  map[string]bool
	isCached bool
}

func (g *DepGraph) Nodes() []*DepNode        { return g.nodes }
func (g *DepGraph) Vars() Vars               { return g.vars }
func (g *DepGraph) Exports() map[string]bool { return g.exports }
func (g *DepGraph) IsCached() bool           { return g.isCached }

type LoadOpt struct {
	Targets         []string
	CommandLineVars []string
	EnvironmentVars []string
	UseCache        bool
}

func Load(makefile string, opt LoadOpt) (*DepGraph, error) {
	startTime := time.Now()
	if makefile == "" {
		makefile = DefaultMakefile()
	}

	if opt.UseCache {
		g := LoadDepGraphCache(makefile, opt.Targets)
		if g != nil {
			return g, nil
		}
	}

	bmk := BootstrapMakefile(opt.Targets)

	content, err := ioutil.ReadFile(makefile)
	if err != nil {
		return nil, err
	}
	mk, err := ParseMakefile(content, makefile)
	if err != nil {
		return nil, err
	}

	for _, stmt := range mk.stmts {
		stmt.show()
	}

	mk.stmts = append(bmk.stmts, mk.stmts...)

	vars := make(Vars)
	err = InitVars(vars, opt.EnvironmentVars, "environment")
	if err != nil {
		return nil, err
	}
	err = InitVars(vars, opt.CommandLineVars, "command line")
	if err != nil {
		return nil, err
	}
	er, err := Eval(mk, vars, opt.UseCache)
	if err != nil {
		return nil, err
	}
	vars.Merge(er.vars)

	LogStats("eval time: %q", time.Since(startTime))
	LogStats("shell func time: %q %d", shellStats.Duration(), shellStats.Count())

	startTime = time.Now()
	db := NewDepBuilder(er, vars)
	LogStats("dep build prepare time: %q", time.Since(startTime))

	startTime = time.Now()
	nodes, err := db.Eval(opt.Targets)
	if err != nil {
		return nil, err
	}
	LogStats("dep build time: %q", time.Since(startTime))
	var readMks []*ReadMakefile
	// Always put the root Makefile as the first element.
	readMks = append(readMks, &ReadMakefile{
		Filename: makefile,
		Hash:     sha1.Sum(content),
		State:    FileExists,
	})
	readMks = append(readMks, er.readMks...)
	return &DepGraph{
		nodes:   nodes,
		vars:    vars,
		readMks: readMks,
		exports: er.exports,
	}, nil
}
