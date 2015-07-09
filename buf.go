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
	"io"
	"sync"
)

var (
	ebufFree = sync.Pool{
		New: func() interface{} { return new(evalBuffer) },
	}
	wbufFree = sync.Pool{
		New: func() interface{} { return new(wordBuffer) },
	}
)

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
	sep bool
}

func (w *ssvWriter) writeWord(word []byte) {
	if w.sep {
		writeByte(w.Writer, ' ')
	}
	w.sep = true
	w.Writer.Write(word)
}

func (w *ssvWriter) writeWordString(word string) {
	if w.sep {
		writeByte(w.Writer, ' ')
	}
	w.sep = true
	io.WriteString(w.Writer, word)
}

func (w *ssvWriter) resetSep() {
	w.sep = false
}

type buffer struct {
	buf       []byte
	bootstrap [64]byte // memory to hold first slice
}

func (b *buffer) Write(data []byte) (int, error) {
	b.buf = append(b.buf, data...)
	return len(data), nil
}

func (b *buffer) WriteByte(c byte) error {
	b.buf = append(b.buf, c)
	return nil
}

func (b *buffer) WriteString(s string) (int, error) {
	b.buf = append(b.buf, []byte(s)...)
	return len(s), nil
}

func (b *buffer) Bytes() []byte  { return b.buf }
func (b *buffer) Len() int       { return len(b.buf) }
func (b *buffer) String() string { return string(b.buf) }

func (b *buffer) Reset() {
	if b.buf == nil {
		b.buf = b.bootstrap[:0]
	}
	b.buf = b.buf[:0]
}

type evalBuffer struct {
	buffer
	ssvWriter
	args [][]byte
}

func newEbuf() *evalBuffer {
	buf := ebufFree.Get().(*evalBuffer)
	buf.Reset()
	return buf
}

func (buf *evalBuffer) release() {
	if cap(buf.Bytes()) > 1024 {
		return
	}
	buf.Reset()
	buf.args = buf.args[:0]
	ebufFree.Put(buf)
}

func (b *evalBuffer) Reset() {
	b.buffer.Reset()
	b.resetSep()
}

func (b *evalBuffer) resetSep() {
	if b.ssvWriter.Writer == nil {
		b.ssvWriter.Writer = &b.buffer
	}
	b.ssvWriter.resetSep()
}

type wordBuffer struct {
	buf   buffer
	words [][]byte
}

func newWbuf() *wordBuffer {
	buf := wbufFree.Get().(*wordBuffer)
	buf.Reset()
	return buf
}

func (buf *wordBuffer) release() {
	if cap(buf.Bytes()) > 1024 {
		return
	}
	buf.Reset()
	wbufFree.Put(buf)
}

func (wb *wordBuffer) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	off := len(wb.buf.buf)
	var cont bool
	if !isWhitespace(rune(data[0])) && len(wb.buf.buf) > 0 {
		cont = !isWhitespace(rune(wb.buf.buf[off-1]))
	}
	ws := newWordScanner(data)
	for ws.Scan() {
		if cont {
			word := wb.words[len(wb.words)-1]
			wb.words = wb.words[:len(wb.words)-1]
			wb.buf.buf = wb.buf.buf[:len(wb.buf.buf)-len(word)]
			var w []byte
			w = append(w, word...)
			w = append(w, ws.Bytes()...)
			wb.writeWord(w)
			cont = false
			continue
		}
		wb.writeWord(ws.Bytes())
	}
	if isWhitespace(rune(data[len(data)-1])) {
		wb.buf.buf = append(wb.buf.buf, ' ')
	}
	return len(data), nil
}

func (wb *wordBuffer) WriteByte(c byte) error {
	_, err := wb.Write([]byte{c})
	return err
}

func (wb *wordBuffer) WriteString(s string) (int, error) {
	return wb.Write([]byte(s))
}

func (wb *wordBuffer) writeWord(word []byte) {
	if len(wb.buf.buf) > 0 {
		wb.buf.buf = append(wb.buf.buf, ' ')
	}
	off := len(wb.buf.buf)
	wb.buf.buf = append(wb.buf.buf, word...)
	wb.words = append(wb.words, wb.buf.buf[off:off+len(word)])
}

func (wb *wordBuffer) writeWordString(word string) {
	wb.writeWord([]byte(word))
}

func (wb *wordBuffer) Reset() {
	wb.buf.Reset()
	wb.words = nil
}

func (wb *wordBuffer) resetSep() {}

func (wb *wordBuffer) Bytes() []byte {
	return wb.buf.Bytes()
}
