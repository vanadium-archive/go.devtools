// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test2

import "v.io/x/devtools/gologcop/testdata/iface2"

// This test makes sure that we only parse an interface or implementation
// package once. The first test in this suite (test1) imports iface2 and we
// so again here. The assignment below (var _ Type2 = ) will fail iface2
// has been parsed twice and there are two sets of data structures for it.

type Type2 struct{}

func (Type2) ReturnsSomething(a int) int {
}

var _ iface2.ReturnsValueInterface = Type2{}
