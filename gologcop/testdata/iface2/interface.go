// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// iface declares an interface used by test packages.
package iface2

type Int int

type Interface1 interface {
	Method1(a int, b string, c map[string]struct{}, e []int) (r1 int, err error)
	Method2(a Int, b string, c map[string]struct{}, e []int)
	Method3() (r1 int, err error)
	Method4() (int, error)
	Method5(err error)
	Method6(a int, b ...interface{})
	Method7(a map[string]struct{}, e []int) (m map[bool]struct{}, err error)
}

type unexportedInterface interface {
	UnexportedInterfaceMethod()
}

type ReturnsValueInterface interface {
	ReturnsSomething(a int) int
}
