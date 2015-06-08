package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
)

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func sendMsg(w io.Writer, data []byte) {
	for len(data) != 0 {
		written, err := w.Write(data)
		if err == io.EOF {
			return
		}
		if err != nil {
			panic(err)
		}
		data = data[written:]
	}
}

func sendInt(w io.Writer, i int) {
	v := int32(i)
	binary.Write(w, binary.LittleEndian, &v)
}

func sendString(w io.Writer, s string) {
	sendInt(w, len(s))
	sendMsg(w, []byte(s))
}

func sendRunners(w io.Writer, runners []runner) {
	sendInt(w, len(runners))
	for _, r := range runners {
		sendString(w, r.output)
		sendString(w, r.cmd)
		sendString(w, r.shell)
		sendInt(w, btoi(r.echo))
		sendInt(w, btoi(r.ignoreError))
	}
}

type ParaResult struct {
	output string
	stdout string
	stderr string
	status int
	signal int
}

func recvInt(r *bufio.Reader) (int, error) {
	var v int32
	err := binary.Read(r, binary.LittleEndian, &v)
	return int(v), err
}

func recvString(r *bufio.Reader) (string, error) {
	l, err := recvInt(r)
	if err != nil {
		return "", err
	}
	buf := make([]byte, l)
	read := 0
	for read < len(buf) {
		r, err := r.Read(buf[read:])
		if err != nil {
			return "", err
		}
		read += r
	}
	return string(buf), nil
}

func recvResult(r *bufio.Reader) (*ParaResult, error) {
	output, err := recvString(r)
	if err != nil {
		return nil, err
	}
	stdout, err := recvString(r)
	if err != nil {
		return nil, err
	}
	stderr, err := recvString(r)
	if err != nil {
		return nil, err
	}
	status, err := recvInt(r)
	if err != nil {
		return nil, err
	}
	signal, err := recvInt(r)
	if err != nil {
		return nil, err
	}
	return &ParaResult{
		output: output,
		stdout: stdout,
		stderr: stderr,
		status: status,
		signal: signal,
	}, nil
}

type ParaWorker struct {
	para     *exec.Cmd
	paraChan chan *ParaResult
	stdin    io.WriteCloser
	stdout   *bufio.Reader
	doneChan chan bool
}

func NewParaWorker(paraChan chan *ParaResult) *ParaWorker {
	bin := filepath.Join(katiDir, "para")
	para := exec.Command(bin, fmt.Sprintf("-j%d", jobsFlag), "--kati")
	stdin, err := para.StdinPipe()
	if err != nil {
		panic(err)
	}
	stdout, err := para.StdoutPipe()
	if err != nil {
		panic(err)
	}
	err = para.Start()
	if err != nil {
		panic(err)
	}
	return &ParaWorker{
		para:     para,
		paraChan: paraChan,
		stdin:    stdin,
		stdout:   bufio.NewReader(stdout),
		doneChan: make(chan bool),
	}
}

func (para *ParaWorker) Run() {
	for {
		r, err := recvResult(para.stdout)
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		para.paraChan <- r
	}
	para.para.Process.Kill()
	para.para.Process.Wait()
	para.doneChan <- true
}

func (para *ParaWorker) Wait() {
	para.stdin.Close()
	<-para.doneChan
}

func (para *ParaWorker) RunCommand(runners []runner) {
	sendRunners(para.stdin, runners)
}
