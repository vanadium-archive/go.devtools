// The following enables go generate to generate the doc.go file.
//
//go:generate go run ../lib/cmdline/testdata/gendoc.go .

package main

func main() {
	root().Main()
}
