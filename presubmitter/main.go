// Command-line tool for various presubmit related functionalities.
//
// Usage:
//    presubmitter [flags] <command>
//
// The presubmitter commands are:
//    query       Query open CLs from Gerrit
//    help        Display help for commands
//
// The presubmitter flags are:
//    -netrc=/var/veyron/.netrc: The path to the .netrc file that stores Gerrit's credentials
//    -url=https://veyron-review.googlesource.com: The base url of the gerrit instance
package main

import (
	"tools/presubmitter/impl"
)

func main() {
	impl.Root().Main()
}
