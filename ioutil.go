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

type wordBuffer struct {
	words [][]byte
	cont  bool
}

func (wb *wordBuffer) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	if isWhitespace(rune(data[0])) {
		wb.cont = false
	}
	ws := newWordScanner(data)
	for ws.Scan() {
		if wb.cont {
			word := wb.words[len(wb.words)-1]
			word = append(word, ws.Bytes()...)
			wb.words[len(wb.words)-1] = word
			wb.cont = false
			continue
		}
		wb.writeWord(ws.Bytes())
	}
	if !isWhitespace(rune(data[len(data)-1])) {
		wb.cont = true
	}
	return len(data), nil
}

func (wb *wordBuffer) writeWord(word []byte) {
	var w []byte
	w = append(w, word...)
	wb.words = append(wb.words, w)
}

func (wb *wordBuffer) writeWordString(word string) {
	wb.writeWord([]byte(word))
}

func (wb *wordBuffer) resetSpace() {}

func (wb *wordBuffer) Bytes() []byte {
	var sp bool
	var b []byte
	for _, word := range wb.words {
		if sp {
			b = append(b, ' ')
		}
		b = append(b, word...)
		sp = true
	}
	return b
}
