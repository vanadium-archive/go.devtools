// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package goutil provides Go wrappers around the Go command-line
// tool.
package goutil

import (
	"bytes"
	"fmt"
	"strings"

	"v.io/jiri/jiri"
)

// List inputs a list of Go package expressions and returns a list of
// Go packages that can be found in the GOPATH and match any of the
// expressions. The implementation invokes 'go list' internally with
// jiriArgs as arguments to the jiri-go subcommand.
func List(jirix *jiri.X, jiriArgs []string, pkgs ...string) ([]string, error) {
	return list(jirix, jiriArgs, "{{.ImportPath}}", pkgs...)
}

// ListDirs inputs a list of Go package expressions and returns a list of
// directories that match the expressions.  The implementation invokes 'go list'
// internally with jiriArgs as arguments to the jiri-go subcommand.
func ListDirs(jirix *jiri.X, jiriArgs []string, pkgs ...string) ([]string, error) {
	return list(jirix, jiriArgs, "{{.Dir}}", pkgs...)
}

func list(jirix *jiri.X, jiriArgs []string, format string, pkgs ...string) ([]string, error) {
	s := jirix.NewSeq()
	args := append([]string{"go"}, jiriArgs...)
	args = append(args, "list", "-f="+format)
	args = append(args, pkgs...)
	var out bytes.Buffer
	if err := s.Capture(&out, &out).Last("jiri", args...); err != nil {
		fmt.Fprintln(jirix.Stderr(), out.String())
		return nil, err
	}
	cleanOut := strings.TrimSpace(out.String())
	if cleanOut == "" {
		return nil, nil
	}
	return strings.Split(cleanOut, "\n"), nil
}
