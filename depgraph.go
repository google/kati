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

// DepGraph represents rules defined in makefiles.
type DepGraph struct {
	nodes       []*DepNode
	vars        Vars
	accessedMks []*accessedMakefile
	exports     map[string]bool
	isCached    bool
}

// Nodes returns all rules.
func (g *DepGraph) Nodes() []*DepNode { return g.nodes }

// Vars returns all variables.
func (g *DepGraph) Vars() Vars { return g.vars }

// Exports returns map for export variables.
func (g *DepGraph) Exports() map[string]bool { return g.exports }

// IsCached indicates the DepGraph is loaded from cache.
func (g *DepGraph) IsCached() bool { return g.isCached }

// LoadReq is a request to load makefile.
type LoadReq struct {
	Makefile         string
	Targets          []string
	CommandLineVars  []string
	EnvironmentVars  []string
	UseCache         bool
	EagerEvalCommand bool
}

// FromCommandLine creates LoadReq from given command line.
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
	mk, err := defaultMakefile()
	if err != nil {
		logf("default makefile: %v", err)
	}
	return LoadReq{
		Makefile:        mk,
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

// Load loads makefile.
func Load(req LoadReq) (*DepGraph, error) {
	startTime := time.Now()
	var err error
	if req.Makefile == "" {
		req.Makefile, err = defaultMakefile()
		if err != nil {
			return nil, err
		}
	}

	if req.UseCache {
		g, err := loadCache(req.Makefile, req.Targets)
		if err == nil {
			return g, nil
		}
	}

	bmk, err := bootstrapMakefile(req.Targets)
	if err != nil {
		return nil, err
	}

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

	logStats("eval time: %q", time.Since(startTime))
	logStats("shell func time: %q %d", shellStats.Duration(), shellStats.Count())

	startTime = time.Now()
	db, err := newDepBuilder(er, vars)
	if err != nil {
		return nil, err
	}
	logStats("dep build prepare time: %q", time.Since(startTime))

	startTime = time.Now()
	nodes, err := db.Eval(req.Targets)
	if err != nil {
		return nil, err
	}
	logStats("dep build time: %q", time.Since(startTime))
	var accessedMks []*accessedMakefile
	// Always put the root Makefile as the first element.
	accessedMks = append(accessedMks, &accessedMakefile{
		Filename: req.Makefile,
		Hash:     sha1.Sum(content),
		State:    fileExists,
	})
	accessedMks = append(accessedMks, er.accessedMks...)
	gd := &DepGraph{
		nodes:       nodes,
		vars:        vars,
		accessedMks: accessedMks,
		exports:     er.exports,
	}
	if req.EagerEvalCommand {
		startTime := time.Now()
		err = evalCommands(nodes, vars)
		if err != nil {
			return nil, err
		}
		logStats("eager eval command time: %q", time.Since(startTime))
	}
	if req.UseCache {
		startTime := time.Now()
		saveCache(gd, req.Targets)
		logStats("serialize time: %q", time.Since(startTime))
	}
	return gd, nil
}

// Loader is the interface that loads DepGraph.
type Loader interface {
	Load(string) (*DepGraph, error)
}

// Saver is the interface that saves DepGraph.
type Saver interface {
	Save(*DepGraph, string, []string) error
}

// LoadSaver is the interface that groups Load and Save methods.
type LoadSaver interface {
	Loader
	Saver
}
