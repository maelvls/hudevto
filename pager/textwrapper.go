// Originally copied from:
// https://github.com/emersion/go-textwrapper/blob/65d89683/wrapper.go

// The MIT License (MIT)
//
// Copyright (c) 2016 emersion
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

// A writer that wraps long text lines to a specified length.
package pager

import (
	"io"
)

type writer struct {
	Len int

	sepBytes []byte
	w        io.Writer
	i        int
}

func (w *writer) Write(b []byte) (N int, err error) {
	to := w.Len - w.i

	for len(b) > to {
		var n int
		n, err = w.w.Write(b[:to])
		if err != nil {
			return
		}
		N += n
		b = b[to:]

		_, err = w.w.Write(w.sepBytes)
		if err != nil {
			return
		}

		w.i = 0
		to = w.Len
	}

	w.i += len(b)

	n, err := w.w.Write(b)
	if err != nil {
		return
	}
	N += n

	return
}

// Returns a writer that splits its input into multiple parts that have the same
// length and adds a separator between these parts.
func New(w io.Writer, sep string, l int) io.Writer {
	return &writer{
		Len:      l,
		sepBytes: []byte(sep),
		w:        w,
	}
}

// Creates a RFC822 text wrapper. It adds a CRLF (ie. \r\n) each 76 characters.
func NewRFC822(w io.Writer) io.Writer {
	return New(w, "\r\n", 76)
}
