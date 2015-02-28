package foo_test

import (
	"testing"

	"v.io/x/ref/lib/modules"
	"v.io/x/ref/lib/testutil/v23tests"
	_ "v.io/x/ref/profiles"
)

func TestHelperProcess(t *testing.T) {
	modules.DispatchInTest()
}
func V23Test(i *v23tests.T) {}
