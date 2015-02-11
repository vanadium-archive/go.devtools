// Package goutil provides Go wrappers around the Go command-line
// tool.
package goutil

import (
	"bytes"
	"fmt"
	"strings"

	"v.io/tools/lib/util"
)

// List inputs a list of Go package expressions and returns a list of
// Go packages that can be found in the GOPATH and match any of the
// expressions. The implementation invokes 'go list' internally.
func List(ctx *util.Context, pkgs []string) ([]string, error) {
	args := []string{"go", "list"}
	args = append(args, pkgs...)
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	if err := ctx.Run().CommandWithOpts(opts, "v23", args...); err != nil {
		fmt.Fprintln(ctx.Stdout(), out.String())
		return nil, err
	}
	cleanOut := strings.TrimSpace(out.String())
	if cleanOut == "" {
		return nil, nil
	}
	return strings.Split(cleanOut, "\n"), nil
}