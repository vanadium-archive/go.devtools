// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// test6 fails the log check, because it incorrectly
// uses LogCall thinking it is LogCallf, which is
// caught as a result of the first argument to
// the returned anonymous function not being a
// pointer.
package test6

import "v.io/x/lib/vlog"

type Type struct{}

func (Type) ReturnsSomething(a int) (b int) {
	defer vlog.LogCall("a: %d", a)("b: %d", &b)
	return 42
}
