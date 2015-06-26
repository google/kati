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
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os/exec"
)

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

type paraConn struct {
	w   io.WriteCloser
	r   *bufio.Reader
	err error
}

func (c *paraConn) sendMsg(data []byte) error {
	if c.err != nil {
		return c.err
	}
	_, err := c.w.Write(data)
	c.err = err
	return err
}

func (c *paraConn) sendInt(i int) error {
	if c.err != nil {
		return c.err
	}
	v := int32(i)
	c.err = binary.Write(c.w, binary.LittleEndian, &v)
	return c.err
}

func (c *paraConn) sendString(s string) error {
	c.sendInt(len(s))
	c.sendMsg([]byte(s))
	return c.err
}

func (c *paraConn) sendRunners(runners []runner) error {
	c.sendInt(len(runners))
	for _, r := range runners {
		c.sendString(r.output)
		c.sendString(r.cmd)
		c.sendString(r.shell)
		c.sendInt(btoi(r.echo))
		c.sendInt(btoi(r.ignoreError))
	}
	return c.err
}

type paraResult struct {
	output string
	stdout string
	stderr string
	status int
	signal int
}

func (c *paraConn) recvInt() (int, error) {
	if c.err != nil {
		return 0, c.err
	}
	var v int32
	c.err = binary.Read(c.r, binary.LittleEndian, &v)
	return int(v), c.err
}

func (c *paraConn) recvString() (string, error) {
	l, err := c.recvInt()
	if err != nil {
		c.err = err
		return "", err
	}
	buf := make([]byte, l)
	_, c.err = io.ReadFull(c.r, buf)
	if c.err != nil {
		return "", c.err
	}
	return string(buf), nil
}

func (c *paraConn) recvResult() (*paraResult, error) {
	output, _ := c.recvString()
	stdout, _ := c.recvString()
	stderr, _ := c.recvString()
	status, _ := c.recvInt()
	signal, _ := c.recvInt()
	if c.err != nil {
		return nil, c.err
	}
	return &paraResult{
		output: output,
		stdout: stdout,
		stderr: stderr,
		status: status,
		signal: signal,
	}, nil
}

type paraWorker struct {
	para     *exec.Cmd
	paraChan chan *paraResult
	c        *paraConn
	doneChan chan bool
}

func newParaWorker(paraChan chan *paraResult, numJobs int, paraPath string) (*paraWorker, error) {
	para := exec.Command(paraPath, fmt.Sprintf("-j%d", numJobs), "--kati")
	stdin, err := para.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := para.StdoutPipe()
	if err != nil {
		return nil, err
	}
	err = para.Start()
	if err != nil {
		return nil, err
	}
	return &paraWorker{
		para:     para,
		paraChan: paraChan,
		c: &paraConn{
			w: stdin,
			r: bufio.NewReader(stdout),
		},
		doneChan: make(chan bool),
	}, nil
}

func (para *paraWorker) Run() error {
	for {
		r, err := para.c.recvResult()
		if err != nil {
			break
		}
		para.paraChan <- r
	}
	para.para.Process.Kill()
	para.para.Process.Wait()
	para.doneChan <- true
	return para.c.err
}

func (para *paraWorker) Wait() error {
	para.c.w.Close()
	<-para.doneChan
	if para.c.err == io.EOF {
		return nil
	}
	return para.c.err
}

func (para *paraWorker) RunCommand(runners []runner) error {
	return para.c.sendRunners(runners)
}
