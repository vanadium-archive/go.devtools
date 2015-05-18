// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// test5 fails the log check, because it does not pass
// return values correctly (they should be passed as
// pointers)
package test5

import "v.io/x/lib/vlog"

type Type struct{}

func (Type) ReturnsSomething(a int) (b int) {
	defer vlog.LogCall(a)(b)
	return 42
}
