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
	jobsFlag = 4

	paraChan := make(chan *ParaResult)
	para := NewParaWorker(paraChan)
	go para.Run()

	numTasks := 100
	for i := 0; i < numTasks; i++ {
		runners := []runner{
			{
				output: fmt.Sprintf("%d", i),
				cmd:    fmt.Sprintf("echo test%d 2>&1", i),
				shell:  "/bin/sh",
			},
		}
		para.RunCommand(runners)
	}

	var started []*ParaResult
	var results []*ParaResult
	for len(started) != numTasks || len(results) != numTasks {
		select {
		case r := <-paraChan:
			fmt.Printf("started=%d finished=%d\n", len(started), len(results))
			if r.status < 0 && r.signal < 0 {
				started = append(started, r)
			} else {
				results = append(results, r)
			}
		}
	}

	para.Wait()
}
