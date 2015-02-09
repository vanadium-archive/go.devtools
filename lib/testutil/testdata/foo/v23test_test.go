package foo_test

import (
	"testing"

	"v.io/core/veyron/lib/modules"
	"v.io/core/veyron/lib/testutil/v23tests"
	_ "v.io/core/veyron/profiles"
)

func TestHelperProcess(t *testing.T) {
	modules.DispatchInTest()
}
func V23Test(i v23tests.T) {}
