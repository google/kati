package main

import (
	"fmt"
	"sort"
	"time"
)

type statsData struct {
	Name    string
	Count   int
	Longest time.Duration
	Total   time.Duration
}

var stats = map[string]statsData{}

func addStats(name string, v Value, t time.Time) {
	if !katiEvalStatsFlag {
		return
	}
	d := time.Now().Sub(t)
	key := fmt.Sprintf("%s:%s", name, v.String())
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
