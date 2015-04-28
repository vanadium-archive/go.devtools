// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// test3 fails the log check because it does not include
// any logging constructs.
package test3

type Type1 struct{}

func (Type1) Method1()    {}
func (Type1) Method2(int) {}

type HalfType2 struct{}

func (HalfType2) Method1() {}

type HalfType3 struct {
	HalfType2
}

func (HalfType3) Method2(int) {}
