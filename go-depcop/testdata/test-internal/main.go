package testinternal

import (
	"veyron.io/tools/go-depcop/testdata/test-internal/internal"
	"veyron.io/tools/go-depcop/testdata/test-internal/internal/child"
)

func main() {
	internal.A()
	child.C()
}
