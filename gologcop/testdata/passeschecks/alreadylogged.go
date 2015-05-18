// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// passeschecks should pass all the log checks, as it includes
// all the necessary logging constructs.
package passeschecks

import "v.io/x/lib/vlog"

type Type1 struct{}

func (Type1) Method1() {
	// random comment
	defer vlog.LogCall("random text")()
}

func (Type1) Method2(int) {
	defer vlog.LogCall()() // random comment
}

func (Type1) NotPartOfInterfaceMethod3() {}

type NotImplementingInterface struct{}

func (NotImplementingInterface) Method1() {}
func (NotImplementingInterface) Method2() {}

type ValueReturningType struct{}

func (ValueReturningType) ReturnsSomething(a int) (b int) {
	defer vlog.LogCall(a)(&b)
	return 42
}

type ValueReturningType2 struct{}

func (ValueReturningType2) ReturnsSomething(a int) (b int) {
	defer vlog.LogCall(a)()
	return 42
}

type UnexportedType struct{}

func (UnexportedType) UnexportedInterfaceMethod() {}

type LogCallfTest struct{}

func (obj LogCallfTest) ReturnsSomething(a int) (b int) {
	// this comment should make no difference
	defer vlog.LogCallf("a: %d", a)("b: %d", &b) // gologcop: DO NOT EDIT, MUST BE FIRST STATEMENT
	// this comment should remain
	return 42
}

func (obj LogCallfTest) AnotherTestForRemove(a int) (b int) {
	// this comment should make no difference
	defer vlog.LogCallf("switch test")("") // gologcop: DO NOT EDIT, MUST BE FIRST STATEMENT
	switch {
	case 4:
		// Another comment
	}
	return 42
}

type NotPartOfInterfacePackageInterface interface {
	NotPartOfInterfacePackageMethod()
}

type NotPartOfInterfacePackageType struct{}

func (NotPartOfInterfacePackageType) NotPartOfInterfacePackageMethod() {}
