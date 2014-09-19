// +build testpackage

// test6 fails the log check, because it incorrectly
// uses LogCall thinking it is LogCallf, which is
// caught as a result of the first argument to
// the returned anonymous function not being a
// pointer.
package test6

import "veyron.io/veyron/veyron2/vlog"

type Type struct{}

func (Type) ReturnsSomething(a int) (b int) {
	defer vlog.LogCall("a: %d", a)("b: %d", &b)
	return 42
}
