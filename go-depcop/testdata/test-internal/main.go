package testinternal

import (
	"v.io/x/devtools/go-depcop/testdata/test-internal/internal"
	"v.io/x/devtools/go-depcop/testdata/test-internal/internal/child"
)

func main() {
	internal.A()
	child.C()
}
