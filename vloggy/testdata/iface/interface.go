// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// iface declares the interface used by test packages.
package iface

type Interface1 interface {
	Method1()
	Method2(a int)
}

type unexportedInterface interface {
	UnexportedInterfaceMethod()
}

type ReturnsValueInterface interface {
	ReturnsSomething(a int) int
}
