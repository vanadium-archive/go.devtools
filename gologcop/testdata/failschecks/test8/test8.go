// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test8

// test8 will fail with since it has no injected statements, but its
// real purpose is to ensure that when an import is added, it is added
// within the paranenthesised import statement and not as an isolated import
// statement.
import (
	"fmt"
)

type Type1 struct{}

func (Type1) Method1() {
	fmt.Println("test")
}
func (Type1) Method2(int) {
	//nologcall
}
