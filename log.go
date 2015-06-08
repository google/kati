package main

import (
	"bytes"
	"fmt"
	"os"
	"runtime/pprof"
)

func LogAlways(f string, a ...interface{}) {
	var buf bytes.Buffer
	buf.WriteString("*kati*: ")
	buf.WriteString(f)
	buf.WriteByte('\n')
	fmt.Printf(buf.String(), a...)
}

func LogStats(f string, a ...interface{}) {
	if !katiLogFlag && !katiStatsFlag {
		return
	}
	LogAlways(f, a...)
}

func Logf(f string, a ...interface{}) {
	if !katiLogFlag {
		return
	}
	LogAlways(f, a...)
}

func Warn(filename string, lineno int, f string, a ...interface{}) {
	f = fmt.Sprintf("%s:%d: warning: %s\n", filename, lineno, f)
	fmt.Printf(f, a...)
}

func WarnNoPrefix(filename string, lineno int, f string, a ...interface{}) {
	f = fmt.Sprintf("%s:%d: %s\n", filename, lineno, f)
	fmt.Printf(f, a...)
}

func Error(filename string, lineno int, f string, a ...interface{}) {
	f = fmt.Sprintf("%s:%d: %s", filename, lineno, f)
	maybeWriteHeapProfile()
	ErrorNoLocation(f, a...)
}

func ErrorNoLocation(f string, a ...interface{}) {
	f = fmt.Sprintf("%s\n", f)
	fmt.Printf(f, a...)
	if cpuprofile != "" {
		pprof.StopCPUProfile()
	}
	maybeWriteHeapProfile()
	dumpStats()
	os.Exit(2)
}
