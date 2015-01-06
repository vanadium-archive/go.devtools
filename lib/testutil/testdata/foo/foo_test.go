package foo

import (
	"testing"
)

func Test1(t *testing.T) {
	if Foo() != "hello" {
		t.Fatalf("that's rude")
	}
}

func Test2(t *testing.T) {}

func Test3(t *testing.T) {}
