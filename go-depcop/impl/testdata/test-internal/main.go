package testinternal

import "tools/go-depcop/impl/testdata/test-internal/internal"
import "tools/go-depcop/impl/testdata/test-internal/internal/child"

func main() {
	internal.A()
	child.C()
}
