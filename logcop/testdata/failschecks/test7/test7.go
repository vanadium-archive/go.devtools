// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// test7 tests whether types with functions receiving
// pointers are implementing the logging constructs
// correctly.
package test7

type PtrType struct{}

func (*PtrType) Method1()    {}
func (*PtrType) Method2(int) {}
