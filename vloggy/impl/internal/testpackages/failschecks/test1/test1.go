// +build testpackage

// test1 should fail the log check because Method1
// prints something before calling LogCall.
package test1

import (
	"fmt"
	"veyron.io/veyron/veyron2/vlog"
)

type Type1 struct{}

func (Type1) Method1() {
	fmt.Println("test")
	defer vlog.LogCall()()
}
func (Type1) Method2(int) {
	//nologcall
}
