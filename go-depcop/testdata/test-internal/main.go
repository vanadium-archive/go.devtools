package testinternal

import (
	"v.io/tools/go-depcop/testdata/test-internal/internal"
	"v.io/tools/go-depcop/testdata/test-internal/internal/child"
)

func main() {
	internal.A()
	child.C()
}
