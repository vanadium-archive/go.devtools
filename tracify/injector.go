// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"go/token"
	"io"
	"io/ioutil"
	"os"
	"text/template"
)

// injectors allow you to rewrite a file adding content at points specified in the source files
// address space.
type injector struct {
	read  int
	r     io.Reader
	w     bytes.Buffer
	fname string
}

func newInjector(fname string) (*injector, error) {
	var err error
	i := &injector{fname: fname}
	i.r, err = os.Open(fname)
	return i, err
}

func (i *injector) copyTo(p token.Position) error {
	toread := p.Offset + 1 - i.read
	i.read += toread
	_, err := io.CopyN(&i.w, i.r, int64(toread))
	return err
}

func (i *injector) inject(p token.Position, content string) error {
	if err := i.copyTo(p); err != nil {
		return err
	}
	_, err := i.w.Write([]byte(content))
	return err
}

func (i *injector) format() error {
	if _, err := io.Copy(&i.w, i.r); err != nil {
		return err
	}
	//out, err := format.Source(i.w.Bytes())
	//if err != nil {
	//	return err
	//}
	out := i.w.Bytes()
	f, err := os.Create(i.fname)
	if err != nil {
		return err
	}
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	return ioutil.WriteFile(i.fname, out, stat.Mode())
}

func (i *injector) execute(p token.Position, t *template.Template, data interface{}) error {
	if err := i.copyTo(p); err != nil {
		return err
	}
	return t.Execute(&i.w, data)
}
