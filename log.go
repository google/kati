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

func Warn(f string, a ...interface{}) {
	var buf bytes.Buffer
	buf.WriteString("warning: ")
	buf.WriteString(f)
	buf.WriteByte('\n')
	fmt.Printf(buf.String(), a...)
}

func Error(f string, a ...interface{}) {
	var buf bytes.Buffer
	buf.WriteString("error: ")
	buf.WriteString(f)
	buf.WriteByte('\n')
	fmt.Printf(buf.String(), a...)
	panic("")
}
