// +build testpackage

// test5 fails the log check, because it does not pass
// return values correctly (they should be passed as
// pointers)
package test5

import "veyron.io/veyron/veyron2/vlog"

type Type struct{}

func (Type) ReturnsSomething(a int) (b int) {
	defer vlog.LogCall(a)(b)
	return 42
}
