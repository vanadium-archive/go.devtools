// +build testpackage

// test4 fails the log check, because it has an executable
// statement in Method1() before the required log statement.
// Type1 is splitted across two files, and this test ensures
// the log checker is able to handle this scenario.
package test4

import (
	"fmt"
	"veyron.io/veyron/veyron2/vlog"
)

type Type1 struct{}

func (Type1) Method1() {
	fmt.Println("test")
	defer vlog.LogCall()()
}
