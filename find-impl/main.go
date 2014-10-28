// The following enables go generate to generate the doc.go file.
//
//go:generate go run ../lib/cmdline/gendoc/main.go

package main

func main() {
	root().Main()
}
