// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY
package external_only_test

import "testing"

import "v.io/core/veyron/lib/modules"

func init() {
	modules.RegisterChild("module", `Oh..`, module)
}

func TestHelperProcess(t *testing.T) {
	modules.DispatchInTest()
}
