// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// test1 should fail the log check because Method1
// prints something before calling LogCall.
package test1

import (
	"fmt"
	"v.io/x/lib/vlog"
)

type Type1 struct{}

func (Type1) Method1() {
	fmt.Println("test")
	defer vlog.LogCall()()
}
func (Type1) Method2(int) {
	//nologcall
}
