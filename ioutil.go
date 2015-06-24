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

import "io"

// ssvWriter is a writer to write space separated values.
type ssvWriter struct {
	w          io.Writer
	needsSpace bool
}

func writeByte(w io.Writer, b byte) error {
	if bw, ok := w.(io.ByteWriter); ok {
		return bw.WriteByte(b)
	}
	_, err := w.Write([]byte{b})
	return err
}

// use io.WriteString to stringWrite.

func (sw *ssvWriter) Write(b []byte) {
	if sw.needsSpace {
		writeByte(sw.w, ' ')
	}
	sw.needsSpace = true
	sw.w.Write(b)
}

func (sw *ssvWriter) WriteString(s string) {
	if sw.needsSpace {
		writeByte(sw.w, ' ')
	}
	sw.needsSpace = true
	io.WriteString(sw.w, s)
}
