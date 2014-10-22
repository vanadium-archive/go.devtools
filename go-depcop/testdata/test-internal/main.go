package testinternal

import (
	"tools/go-depcop/testdata/test-internal/internal"
	"tools/go-depcop/testdata/test-internal/internal/child"
)

func main() {
	internal.A()
	child.C()
}
