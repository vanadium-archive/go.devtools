// The following enables go generate to generate the doc.go file.
//
//go:generate go run $VANADIUM_ROOT/release/go/src/v.io/lib/cmdline/testdata/gendoc.go .

package main

func main() {
	substituteVarsInFlags()
	root().Main()
}
