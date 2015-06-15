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
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"
)

type traceEventT struct {
	mu  sync.Mutex
	f   io.WriteCloser
	t0  time.Time
	pid int
}

const (
	traceEventMain = iota + 1
	traceEventFindCache
)

var traceEvent traceEventT

func (t *traceEventT) start(f io.WriteCloser) {
	t.f = f
	t.t0 = time.Now()
	fmt.Fprint(t.f, "[ ")
}

func (t *traceEventT) enabled() bool {
	return t.f != nil
}

func (t *traceEventT) stop() {
	fmt.Fprint(t.f, "\n]\n")
	t.f.Close()
}

type event struct {
	name, v string
	tid     int
	t       time.Time
	emit    bool
}

func (t *traceEventT) begin(name string, v string, tid int) event {
	t.mu.Lock()
	defer t.mu.Unlock()
	var e event
	e.tid = tid
	e.t = time.Now()
	if t.f != nil || katiEvalStatsFlag {
		e.name = name
		e.v = v
	}
	if t.f != nil {
		e.emit = name == "include" || name == "shell" || name == "findcache"
		if e.emit {
			if t.pid == 0 {
				t.pid = os.Getpid()
			} else {
				fmt.Fprintf(t.f, ",\n")
			}
			ts := e.t.Sub(t.t0)
			fmt.Fprintf(t.f, `{"pid":%d,"tid":%d,"ts":%d,"ph":"B","cat":%q,"name":%q,"args":{}}`,
				t.pid,
				e.tid,
				ts.Nanoseconds()/1e3,
				e.name,
				e.v,
			)
		}
	}
	return e
}

func (t *traceEventT) end(e event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.f != nil {
		now := time.Now()
		ts := now.Sub(t.t0)
		if e.emit {
			fmt.Fprint(t.f, ",\n")
			fmt.Fprintf(t.f, `{"pid":%d,"tid":%d,"ts":%d,"ph":"E","cat":%q,"name":%q}`,
				t.pid,
				e.tid,
				ts.Nanoseconds()/1e3,
				e.name,
				e.v,
			)
		}
	}
	addStats(e.name, e.v, e.t)
}

type statsData struct {
	Name    string
	Count   int
	Longest time.Duration
	Total   time.Duration
}

var stats = map[string]statsData{}

func addStats(name, v string, t time.Time) {
	if !katiEvalStatsFlag {
		return
	}
	d := time.Since(t)
	key := fmt.Sprintf("%s:%s", name, v)
	s := stats[key]
	if d > s.Longest {
		s.Longest = d
	}
	s.Total += d
	s.Count++
	stats[key] = s
}

func dumpStats() {
	if !katiEvalStatsFlag {
		return
	}
	var sv byTotalTime
	for k, v := range stats {
		v.Name = k
		sv = append(sv, v)
	}
	sort.Sort(sv)
	fmt.Println("count,longest(ns),total(ns),longest,total,name")
	for _, s := range sv {
		fmt.Printf("%d,%d,%d,%v,%v,%s\n", s.Count, s.Longest, s.Total, s.Longest, s.Total, s.Name)
	}
}

type byTotalTime []statsData

func (b byTotalTime) Len() int      { return len(b) }
func (b byTotalTime) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b byTotalTime) Less(i, j int) bool {
	return b[i].Total > b[j].Total
}
