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
	"fmt"
	"io/ioutil"
	"strings"
	"time"
)

type DepGraph struct {
	nodes       []*DepNode
	vars        Vars
	accessedMks []*accessedMakefile
	exports     map[string]bool
	isCached    bool
}

func (g *DepGraph) Nodes() []*DepNode        { return g.nodes }
func (g *DepGraph) Vars() Vars               { return g.vars }
func (g *DepGraph) Exports() map[string]bool { return g.exports }
func (g *DepGraph) IsCached() bool           { return g.isCached }

type LoadReq struct {
	Makefile        string
	Targets         []string
	CommandLineVars []string
	EnvironmentVars []string
	UseCache        bool
}

func FromCommandLine(cmdline []string) LoadReq {
	var vars []string
	var targets []string
	for _, arg := range cmdline {
		if strings.IndexByte(arg, '=') >= 0 {
			vars = append(vars, arg)
			continue
		}
		targets = append(targets, arg)
	}
	return LoadReq{
		Makefile:        defaultMakefile(),
		Targets:         targets,
		CommandLineVars: vars,
	}
}

func initVars(vars Vars, kvlist []string, origin string) error {
	for _, v := range kvlist {
		kv := strings.SplitN(v, "=", 2)
		logf("%s var %q", origin, v)
		if len(kv) < 2 {
			return fmt.Errorf("A weird %s variable %q", origin, kv)
		}
		vars.Assign(kv[0], &recursiveVar{
			expr:   literal(kv[1]),
			origin: origin,
		})
	}
	return nil
}

func Load(req LoadReq) (*DepGraph, error) {
	startTime := time.Now()
	if req.Makefile == "" {
		req.Makefile = defaultMakefile()
	}

	if req.UseCache {
		g := LoadDepGraphCache(req.Makefile, req.Targets)
		if g != nil {
			return g, nil
		}
	}

	bmk := bootstrapMakefile(req.Targets)

	content, err := ioutil.ReadFile(req.Makefile)
	if err != nil {
		return nil, err
	}
	mk, err := parseMakefile(content, req.Makefile)
	if err != nil {
		return nil, err
	}

	for _, stmt := range mk.stmts {
		stmt.show()
	}

	mk.stmts = append(bmk.stmts, mk.stmts...)

	vars := make(Vars)
	err = initVars(vars, req.EnvironmentVars, "environment")
	if err != nil {
		return nil, err
	}
	err = initVars(vars, req.CommandLineVars, "command line")
	if err != nil {
		return nil, err
	}
	er, err := eval(mk, vars, req.UseCache)
	if err != nil {
		return nil, err
	}
	vars.Merge(er.vars)

	LogStats("eval time: %q", time.Since(startTime))
	LogStats("shell func time: %q %d", shellStats.Duration(), shellStats.Count())

	startTime = time.Now()
	db := newDepBuilder(er, vars)
	LogStats("dep build prepare time: %q", time.Since(startTime))

	startTime = time.Now()
	nodes, err := db.Eval(req.Targets)
	if err != nil {
		return nil, err
	}
	LogStats("dep build time: %q", time.Since(startTime))
	var accessedMks []*accessedMakefile
	// Always put the root Makefile as the first element.
	accessedMks = append(accessedMks, &accessedMakefile{
		Filename: req.Makefile,
		Hash:     sha1.Sum(content),
		State:    fileExists,
	})
	accessedMks = append(accessedMks, er.accessedMks...)
	return &DepGraph{
		nodes:       nodes,
		vars:        vars,
		accessedMks: accessedMks,
		exports:     er.exports,
	}, nil
}
