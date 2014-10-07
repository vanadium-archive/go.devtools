// The following enables go generate to generate the doc.go file.
//
//go:generate ./gendoc.sh

package main

import (
	"tools/presubmitter/impl"
)

func main() {
	impl.Root().Main()
}
