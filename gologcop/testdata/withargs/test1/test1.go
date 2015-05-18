// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test1

import "v.io/x/devtools/gologcop/testdata/iface2"

type Type1 struct{}

func (Type1) Method1(a int, b string, c map[string]struct{}, e []int) (r1 int, err error) {
	return 1, nil
}

func (Type1) Method2(a iface2.Int, b string, c map[string]struct{}, e []int) {
}

func (Type1) Method3() (r1 int, err error) {
	return 3, nil
}

func (Type1) Method4() (int, error) {
	return 4, nil
}

func (Type1) Method5(err error) {
}

func (Type1) Method6(a int, b ...interface{}) {
}

func (Type1) Method7(a map[string]struct{}, e []int) (m map[bool]struct{}, err error) {
}
