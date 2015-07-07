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
	"bytes"
	"io"
	"sync"
)

var bufFree = sync.Pool{
	New: func() interface{} { return new(buffer) },
}

func writeByte(w io.Writer, b byte) error {
	if bw, ok := w.(io.ByteWriter); ok {
		return bw.WriteByte(b)
	}
	_, err := w.Write([]byte{b})
	return err
}

// use io.WriteString to stringWrite.

type ssvWriter struct {
	io.Writer
	space bool
}

func (w *ssvWriter) writeWord(word []byte) {
	if w.space {
		writeByte(w.Writer, ' ')
	}
	w.space = true
	w.Writer.Write(word)
}

func (w *ssvWriter) writeWordString(word string) {
	if w.space {
		writeByte(w.Writer, ' ')
	}
	w.space = true
	io.WriteString(w.Writer, word)
}

func (w *ssvWriter) resetSpace() {
	w.space = false
}

type buffer struct {
	bytes.Buffer
	ssvWriter
	args [][]byte
}

func newBuf() *buffer {
	buf := bufFree.Get().(*buffer)
	buf.Reset()
	return buf
}

func freeBuf(buf *buffer) {
	if cap(buf.Bytes()) > 1024 {
		return
	}
	buf.Reset()
	buf.args = buf.args[:0]
	bufFree.Put(buf)
}

func (b *buffer) Reset() {
	b.Buffer.Reset()
	b.resetSpace()
}

func (b *buffer) resetSpace() {
	if b.ssvWriter.Writer == nil {
		b.ssvWriter.Writer = &b.Buffer
	}
	b.ssvWriter.resetSpace()
}
