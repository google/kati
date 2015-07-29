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
	// add new ones to use new goroutine.
)

var traceEvent traceEventT

// TraceEventStart starts trace event.
func TraceEventStart(f io.WriteCloser) {
	traceEvent.start(f)
}

// TraceEventStop stops trace event.
func TraceEventStop() {
	traceEvent.stop()
}

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

func (t *traceEventT) begin(name string, v Value, tid int) event {
	var e event
	e.tid = tid
	e.t = time.Now()
	if t.f != nil || EvalStatsFlag {
		e.name = name
		e.v = v.String()
	}
	if t.f != nil {
		e.emit = name == "include" || name == "shell"
		if e.emit {
			t.emit("B", e, e.t.Sub(t.t0))
		}
	}
	return e
}

func (t *traceEventT) emit(ph string, e event, ts time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.pid == 0 {
		t.pid = os.Getpid()
	} else {
		fmt.Fprintf(t.f, ",\n")
	}
	fmt.Fprintf(t.f, `{"pid":%d,"tid":%d,"ts":%d,"ph":%q,"cat":%q,"name":%q,"args":{}}`,
		t.pid,
		e.tid,
		ts.Nanoseconds()/1e3,
		ph,
		e.name,
		e.v,
	)
}

func (t *traceEventT) end(e event) {
	if t.f != nil {
		if e.emit {
			t.emit("E", e, time.Since(t.t0))
		}
	}
	stats.add(e.name, e.v, e.t)
}

type statsData struct {
	Name    string
	Count   int
	Longest time.Duration
	Total   time.Duration
}

type statsT struct {
	mu   sync.Mutex
	data map[string]statsData
}

var stats = &statsT{
	data: make(map[string]statsData),
}

func (s *statsT) add(name, v string, t time.Time) {
	if !EvalStatsFlag {
		return
	}
	d := time.Since(t)
	key := fmt.Sprintf("%s:%s", name, v)
	s.mu.Lock()
	sd := s.data[key]
	if d > sd.Longest {
		sd.Longest = d
	}
	sd.Total += d
	sd.Count++
	s.data[key] = sd
	s.mu.Unlock()
}

// DumpStats dumps statistics collected if EvalStatsFlag is set.
func DumpStats() {
	if !EvalStatsFlag {
		return
	}
	var sv byTotalTime
	for k, v := range stats.data {
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

type shellStatsT struct {
	mu       sync.Mutex
	duration time.Duration
	count    int
}

var shellStats = &shellStatsT{}

func (s *shellStatsT) add(d time.Duration) {
	s.mu.Lock()
	s.duration += d
	s.count++
	s.mu.Unlock()
}

func (s *shellStatsT) Duration() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.duration
}

func (s *shellStatsT) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}
