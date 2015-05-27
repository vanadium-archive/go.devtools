// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// test4 fails the log check, because it has an executable
// statement in Method1() before the required log statement.
// Type1 is splitted across two files, and this test ensures
// the log checker is able to handle this scenario.
package test4

import (
	"fmt"
	"v.io/x/ref/lib/apilog"
)

type Type1 struct{}

func (Type1) Method1() {
	fmt.Println("test")
	defer vlog.LogCall()()
}
