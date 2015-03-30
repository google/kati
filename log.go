package main

import (
	"bytes"
	"fmt"
)

func Log(f string, a ...interface{}) {
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

func Error(f string, a ...interface{}) {
	var buf bytes.Buffer
	buf.WriteString("error: ")
	buf.WriteString(f)
	buf.WriteByte('\n')
	fmt.Printf(buf.String(), a...)
	panic("")
}
