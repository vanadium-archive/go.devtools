package main

import (
	"fmt"
	a "tools/go-depcop/impl/internal/testpackages/test-a"
)

func main() {
	fmt.Println("B")
	a.A()
}
