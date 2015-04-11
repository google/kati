package main

import (
	"bytes"
	"fmt"
	"os"
	"runtime/pprof"
)

func Log(f string, a ...interface{}) {
	if !katiLogFlag {
		return
	}

	var buf bytes.Buffer
	buf.WriteString("*kati*: ")
	buf.WriteString(f)
	buf.WriteByte('\n')
	fmt.Printf(buf.String(), a...)
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
	ErrorNoLocation(f, a...)
}

func ErrorNoLocation(f string, a ...interface{}) {
	f = fmt.Sprintf("%s\n", f)
	fmt.Printf(f, a...)
	if cpuprofile != "" {
		pprof.StopCPUProfile()
	}
	os.Exit(2)
}
