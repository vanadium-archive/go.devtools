package testinternal

import "tools/go-depcop/impl/internal/testpackages/test-internal/internal"
import "tools/go-depcop/impl/internal/testpackages/test-internal/internal/child"

func main() {
	internal.A()
	child.C()
}
