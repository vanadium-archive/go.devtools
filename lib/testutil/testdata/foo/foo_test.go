package foo

import (
	"testing"
)

func TestFoo(t *testing.T) {
	if Foo() != "hello" {
		t.Fatalf("that's rude")
	}
}
