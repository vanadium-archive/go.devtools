// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go . -help

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/build"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"v.io/x/lib/cmdline"
	"v.io/x/lib/envvar"
)

func main() {
	cmdline.Main(cmdBendroid)
}

// TODO(mattr): As much as possible support all the same flags as 'go test'.
// We should especially support -x.
var (
	bench = flag.String("bench", "",
		"Run benchmarks matching the regular expression."+
			" By default, no benchmarks run. To run all benchmarks,"+
			" use '-bench .' or '-bench=.'.")
	benchmem = flag.Bool("benchmem", false,
		"Print memory allocation statistics for benchmarks.")
	benchtime = flag.Duration("benchtime", time.Second,
		"Print memory allocation statistics for benchmarks.")
	run = flag.String("run", "",
		"Run only those tests and examples matching the regular"+
			" expression.")
	verbose = flag.Bool("v", false,
		"Verbose output: log all tests as they are run. Also print all"+
			" text from Log and Logf calls even if the test succeeds.")
	compileOnly = flag.Bool("c", false,
		"Compile the test binary to pkg.test but do not run it"+
			" (where pkg is the last element of the package's import path)."+
			" The file name can be changed with the -o flag.")
	outName = flag.String("o", "",
		"Compile the test binary to the named file."+
			" The test still runs (unless -c is specified).")
	work = flag.Bool("work", false,
		"print the name of the temporary work directory and"+
			" do not delete it when exiting.")
	tags = flag.String("tags", "",
		"a list of build tags to consider satisfied during the build."+
			" For more information about build tags, see the description of"+
			" build constraints in the documentation for the go/build package.")
)

var cmdBendroid = &cmdline.Command{
	Name:  "bendroid",
	Short: "Execute tests and benchmarks on android phones.",
	Long: `
bendroid attempts to emulate the behavior of go test, but running the tests
and benchmarks on an android device.

Note that currently we support only a small subset of the flags allowed to
'go test'.

We depend on gradle and adb, so those tools should be in your path.

You should also set relevant CGO envrionment variables (for example pointing at the
ndk cc and gcc) see: https://golang.org/cmd/cgo/.  Unlike gomobile, we don't
set them for you.
`,
	ArgsName: "[-c] [build and test flags] [packages] [flags for test binary]",
	Runner:   cmdline.RunnerFunc(bendroid),
}

func bendroid(env *cmdline.Env, args []string) error {
	for _, bin := range []string{"adb", "gradle"} {
		if path, err := exec.LookPath(bin); err != nil || path == "" {
			fmt.Fprintln(env.Stderr, "%s not found, it must be in your path.", bin)
			os.Exit(1)
		}
	}
	var pkgFlags, pkgArgs []string
	pkgArgs = args
	for i, a := range args {
		if strings.HasPrefix(a, "-") {
			pkgArgs, pkgFlags = args[:i], args[i:]
			break
		}
	}
	packages, err := readPackages(env, pkgArgs)
	if err != nil {
		return err
	}
	npackages := len(packages)
	if npackages > 1 && *compileOnly {
		fmt.Fprintln(env.Stderr, "cannot use -c flag with multiple packages")
		os.Exit(1)
	}
	if npackages > 1 && len(*outName) != 0 {
		fmt.Fprintln(env.Stderr, "cannot use -o flag with multiple packages")
		os.Exit(1)
	}
	runs := make([]*testrun, len(packages))
	defer func() {
		for _, r := range runs {
			if r != nil {
				r.clean()
			}
		}
	}()
	for i, p := range packages {
		if runs[i], err = newTestrun(env, p, pkgFlags); err != nil {
			return err
		}
	}
	if *compileOnly {
		return runs[0].build()
	}

	done := make(chan error)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)

	buildReady := make([]chan error, npackages)
	for i := range buildReady {
		buildReady[i] = make(chan error, 1)
	}

	go func() {
		buildSema := make(chan struct{}, runtime.NumCPU())
		for idx := range runs {
			buildSema <- struct{}{}
			go func(idx int) {
				buildReady[idx] <- runs[idx].build()
				<-buildSema
			}(idx)
		}
	}()

	go func() {
		errors := 0
		for idx, r := range runs {
			var duration time.Duration
			if err = <-buildReady[idx]; err == nil {
				duration, err = r.run()
			}
			switch err {
			case nil:
				fmt.Fprintf(env.Stdout, "ok\t%s\t%s\n", r.BuildPkg.ImportPath, duration)
			case errNoTests:
				fmt.Fprintf(env.Stdout, "?\t%s\t[no test files]\n", r.BuildPkg.ImportPath)
			case errFailedRun:
				fmt.Fprintf(env.Stdout, "FAIL\t%s\t%s\n", r.BuildPkg.ImportPath, duration)
				errors++
			default:
				fmt.Fprintf(env.Stdout, "FAIL\t%s\t[setup failed]\n", r.BuildPkg.ImportPath)
				errors++
			}
		}
		if errors > 0 {
			done <- fmt.Errorf("There were failed tests.")
		}
		done <- nil
	}()

	select {
	case <-sigs:
		return fmt.Errorf("interrupted.")
	case err := <-done:
		return err
	}
}

// readPackages resolves the user-supplied package patterns to a list of actual packages.
// We just call out to 'go list' for this since there is actually a lot of subtlety
// in resolving the patterns.
func readPackages(env *cmdline.Env, args []string) ([]*build.Package, error) {
	buf := &bytes.Buffer{}
	opts := []string{"list", "-json"}
	if *tags != "" {
		opts = append(opts, "-tags", *tags)
	}
	cmd := exec.Command("go", append(opts, args...)...)
	cmd.Env = envvar.MapToSlice(env.Vars)
	cmd.Stderr = env.Stderr
	cmd.Stdout = buf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("Could not list packages: %v", err)
	}
	dec := json.NewDecoder(buf)
	packages := []*build.Package{}
	for {
		var pkg build.Package
		if err := dec.Decode(&pkg); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		packages = append(packages, &pkg)
	}
	return packages, nil
}

type funcref struct {
	Name, Package string
}

type testrun struct {
	Env                      *cmdline.Env
	BuildPkg                 *build.Package
	AndroidPackage           string
	TmpDir                   string
	Tests                    []funcref
	Benchmarks               []funcref
	Examples                 []funcref
	TestMainPackage          string
	BaseDir, ExtDir, MainDir string
	BasePkg, ExtPkg, MainPkg string
	FuncImports              []string
	Flags                    []string
	apk                      string
	cleanup                  []string

	// inplace is true if we are using the directory of the
	// tested package to rewrite code, or if we are using a
	// separate directory.  We usually just generate code
	// directly into the packages directory, but if the package
	// being tested is a 'main' package we can't do that.  main
	// packages can't be imported by other packages (in this
	// case the package we generate to be the test main) so
	// we have to copy the package to a new directory and
	// rewrite every file to be some non-main package.
	inplace bool
}

func newTestrun(env *cmdline.Env, pkg *build.Package, flags []string) (*testrun, error) {
	t := &testrun{
		BuildPkg: pkg,
		Env:      env,
	}

	for _, fname := range []string{"bench", "benchmem", "benchtime", "run", "v"} {
		t.Flags = append(t.Flags, "-test."+fname+"="+flag.Lookup(fname).Value.String())
	}
	t.Flags = append(t.Flags, flags...)

	// Compute the android package name.
	parts := strings.Split(pkg.ImportPath, "/")
	domainparts := strings.Split(parts[0], ".")
	l := len(domainparts)
	// reverse the domain java style.
	for i, j := 0, l-1; i < l/2; i, j = i+1, j-1 {
		domainparts[i], domainparts[j] = domainparts[j], domainparts[i]
	}
	parts[0] = strings.Join(domainparts, ".")
	t.AndroidPackage = strings.Join(parts, ".")

	var err error
	if pkg.Name == "main" {
		// We need to treat main packages specially.  See the comment next to
		// the testrun.inplace.
		if t.BaseDir, t.BasePkg, err = pkgDir(pkg.Dir, "bendroid"); err != nil {
			return nil, err
		}
		t.cleanup = append(t.cleanup, t.BaseDir)
	} else {
		t.BaseDir, t.BasePkg = pkg.Dir, pkg.Name
		t.inplace = true
	}
	if len(pkg.XTestGoFiles) > 0 {
		if t.ExtDir, t.ExtPkg, err = pkgDir(t.BaseDir, "bendroidext"); err != nil {
			return nil, err
		}
		t.cleanup = append(t.cleanup, t.ExtDir)
	}
	if t.MainDir, t.MainPkg, err = pkgDir(t.BaseDir, "bendroidmain"); err != nil {
		return nil, err
	}
	t.cleanup = append(t.cleanup, t.MainDir)
	if len(*outName) > 0 {
		t.apk = *outName
	} else if *compileOnly {
		t.apk = pkg.Name + ".apk"
	} else {
		t.apk = filepath.Join(t.MainDir, pkg.Name+".apk")
	}
	return t, nil
}

func (t *testrun) clean() {
	if !*work {
		for _, item := range t.cleanup {
			os.RemoveAll(item)
		}
	}
	exec.Command("adb", "uninstall", t.AndroidPackage).Run()
}

func pkgDir(base, pfx string) (dir, pkg string, err error) {
	dir, err = ioutil.TempDir(base, pfx)
	return dir, filepath.Base(dir), err
}
