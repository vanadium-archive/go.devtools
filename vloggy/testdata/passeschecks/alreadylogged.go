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
	defer vlog.LogCall()()
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
	defer vlog.LogCallf("a: %d", a)("b: %d", &b)
	return 42
}

type NotPartOfInterfacePackageInterface interface {
	NotPartOfInterfacePackageMethod()
}

type NotPartOfInterfacePackageType struct{}

func (NotPartOfInterfacePackageType) NotPartOfInterfacePackageMethod() {}
