package main

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestPara(t *testing.T) {
	cwd, err := filepath.Abs(".")
	if err != nil {
		panic(err)
	}
	katiDir = cwd

	paraChan := make(chan *ParaResult)
	para := NewParaWorker(paraChan)
	go para.Run()

	num_tasks := 100
	for i := 0; i < num_tasks; i++ {
		runners := []runner{
			{
				output: fmt.Sprintf("%d", i),
				cmd:    fmt.Sprintf("echo test%d 2>&1", i),
				shell:  "/bin/sh",
			},
		}
		para.RunCommand(runners)
	}

	var results []*ParaResult
	for len(results) != num_tasks {
		select {
		case r := <-paraChan:
			results = append(results, r)
		}
	}

	para.Wait()
}
