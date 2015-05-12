// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// iface declares the interface used by test packages.
package iface

import "v.io/v23/context"

type Interface1 interface {
	Method1()
	Method2(a int)
	Method3(ctx *context.T, b int)
}
