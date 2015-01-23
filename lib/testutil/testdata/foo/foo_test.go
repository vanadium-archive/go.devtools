package foo_test

import (
	"testing"

	"v.io/tools/lib/testutil/testdata/foo"
)

func Test1(t *testing.T) {
	if foo.Foo() != "hello" {
		t.Fatalf("that's rude")
	}
}
