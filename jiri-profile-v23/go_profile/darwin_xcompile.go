// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package go_profile

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
	"v.io/jiri/profiles/profilesutil"
)

type absoluteLink struct {
	source, target string
}

const (
	// TODO(cnicolaou): have these version controlled - i.e. the
	// version specified in the target will be used to chose the
	// clang/llvm etc versions.
	clangSrc    = "cfe-3.6.2.src"
	llvmSrc     = "llvm-3.6.2.src"
	binutilsSrc = "binutils-2.25"
	binutilsURL = "http://ftp.gnu.org/gnu/binutils/" + binutilsSrc + ".tar.gz"
	clangURL    = "http://llvm.org/releases/3.6.2/" + clangSrc + ".tar.xz"
	llvmURL     = "http://llvm.org/releases/3.6.2/" + llvmSrc + ".tar.xz"
)

const compilerTemplate = `#!/bin/bash
JIRI_ROOT=${JIRI_ROOT:={{.JiriRoot}}}
compiler={{.Compiler}}
export PATH={{.ClangBin}}:$PATH:
exec $compiler --target={{.Target}} -B{{.BinutilsBin}} --sysroot={{.Sysroot}}  -isysroot {{.ISysroot}}  "$@"
`

type compilerInfo struct {
	JiriRoot string
	Compiler string
	ClangBin string
	Target   string
	// Sysroot applies to libraries and is specified using --sysroot=<dir>
	Sysroot string
	// ISysroot applies to header files and is specified using -isysroot <dir>
	ISysroot    string
	BinutilsBin string
}

func useLLVM(jirix *jiri.X, m *Manager, root jiri.RelPath, target profiles.Target, action profiles.Action) (bindir string, env []string, e error) {
	targetABI := ""
	targetDir := target.TargetSpecificDirname()
	switch target.Arch() {
	case "arm":
		targetABI = "arm-linux-gnueabi"
	default:
		return "", nil, fmt.Errorf("Arch %q is not yet supported for llvm", target.Arch())
	}

	// Directory structure is a follows:
	// - profiles/llvm contains the llvm source and build.<llvm source> directories
	// - profiles/cout/<target>/llvm contains the installed binaries
	//   and configured copy of the sysroot.
	// The detailed directory structure underneath these is constructed to
	// support AtomicActions on the expensive operations, in particular,
	// each unique sysroot argument gets a directory to itself.

	src := m.root.Join("llvm")
	inst := m.root.Join("cout", targetDir, "llvm")

	if action == profiles.Uninstall {
		err := jirix.NewSeq().
			RemoveAll(src.Abs(jirix)).
			RemoveAll(inst.Abs(jirix)).Done()
		return "", nil, err
	}

	if len(goSysrootFlag) == 0 {
		return "", nil, fmt.Errorf("sysroot argument is missing")
	}

	sysrootImage := filepath.Base(goSysrootFlag)
	// The following are all AtomicAction directories.
	binutilsBin := inst.Join(binutilsSrc)
	clangBuildSrc := src.Join("build." + llvmSrc)
	// Copy the tarball into here.
	sysrootTarballDir := inst.Join(filepath.Join("sysroot", sysrootImage))
	// Unpack and configure the tarball into here, it's nested under the
	// tarball directory since it makes no sense to keep it around if
	// the tarball has changed.
	sysrootDir := sysrootTarballDir.Join("root")

	absSrc := src.Abs(jirix)
	for _, url := range []string{binutilsURL, llvmURL, clangURL} {
		if err := downloadAndUnpackFile(jirix, url, absSrc); err != nil {
			return "", nil, err
		}
	}

	absBinutilsBin := binutilsBin.Abs(jirix)
	binutilsFn := func() error {
		return jirix.NewSeq().Pushd(src.Abs(jirix)).
			Pushd(binutilsSrc).
			MkdirAll(absBinutilsBin, profilesutil.DefaultDirPerm).
			Run("./configure", "--target="+targetABI, "--program-prefix=",
			"--prefix="+absBinutilsBin, "--with-sysroot=yes").
			Run("make", "-j8").
			Last("make", "install")
	}
	if err := profilesutil.AtomicAction(jirix, binutilsFn, absBinutilsBin, "Build and install binutils"); err != nil {
		return "", nil, err
	}

	absClangBuildSrc := clangBuildSrc.Abs(jirix)
	absClangSrc := src.Join(clangSrc).Abs(jirix)
	absLLVMSrc := src.Join(llvmSrc).Abs(jirix)
	clangFn := func() error {
		return jirix.NewSeq().MkdirAll(absClangBuildSrc, profilesutil.DefaultDirPerm).
			Pushd(absClangBuildSrc).
			Run("cmake", "-GUnix Makefiles", "-DCMAKE_INSTALL_PREFIX="+inst.Abs(jirix),
			"-DLLVM_TARGETS_TO_BUILD=ARM",
			"-DLLVM_EXTERNAL_CLANG_SOURCE_DIR="+absClangSrc, absLLVMSrc).
			Run("make", "-j8").
			Last("make", "install")
	}
	if err := profilesutil.AtomicAction(jirix, clangFn, absClangBuildSrc, "Build and install clang"); err != nil {
		return "", nil, err
	}

	s := jirix.NewSeq()

	absClangBin := inst.Join("bin").Abs(jirix)
	// Write out helper scripts to run clang with the appropriate PATH etc.
	ci := &compilerInfo{
		JiriRoot:    jirix.Root,
		ClangBin:    inst.Join("bin").Symbolic(),
		Target:      targetABI + "hf",
		Sysroot:     sysrootDir.Symbolic(),
		BinutilsBin: binutilsBin.Join("bin").Symbolic(),
	}
	ci.ISysroot = filepath.Join(sysrootDir.Symbolic(), "usr", "lib", "gcc", ci.Target)
	tmpl := template.New("compiler scripts")
	tmpl, err := tmpl.Parse(compilerTemplate)
	if err != nil {
		return "", nil, fmt.Errorf("Failed to parse template %v\n", err)
	}

	for _, compiler := range []string{"clang", "clang++"} {
		ci.Compiler = filepath.Join(ci.ClangBin, compiler)
		var out bytes.Buffer
		if err := tmpl.Execute(&out, &ci); err != nil {
			return "", nil, fmt.Errorf("Failed to execute template for %s: %v\n", compiler, err)
		}
		ofile := filepath.Join(absClangBin, "go-"+targetABI+"-"+compiler)
		if err := s.RemoveAll(ofile).WriteFile(ofile, out.Bytes(), os.FileMode(0755)).Done(); err != nil {
			return "", nil, fmt.Errorf("Failed to write: %s: %v", ofile, err)
		}
	}

	absSysrootTarballDir := sysrootTarballDir.Abs(jirix)
	sysrootTGZ := filepath.Join(absSysrootTarballDir, "sysroot.tar.gz")
	tarArgs := []string{"czf", sysrootTGZ}
	for _, k := range strings.Split(goSysrootDirs, ":") {
		if filepath.IsAbs(k) {
			k, _ = filepath.Rel(string(filepath.Separator), k)
		}
		tarArgs = append(tarArgs, k)
	}
	sysrootCopyFn := func() error {
		return jirix.NewSeq().
			MkdirAll(absSysrootTarballDir, profilesutil.DefaultDirPerm).
			Pushd(goSysrootFlag).
			Last("tar", tarArgs...)
	}
	if err := profilesutil.AtomicAction(jirix, sysrootCopyFn, absSysrootTarballDir, "Copy sysroot"); err != nil {
		return "", nil, err
	}

	absSysroot := filepath.Join(absSysrootTarballDir, "root")

	sysrootConfigFn := func() error {
		s := jirix.NewSeq()
		sysrootRoot := filepath.Join(absSysrootTarballDir, "root")
		if err := s.Pushd(absSysrootTarballDir).
			RemoveAll("root").
			MkdirAll("root", profilesutil.DefaultDirPerm).
			Pushd("root").
			Last("tar", "zxf", sysrootTGZ); err != nil {
			return err
		}
		return rewriteSysroot(jirix, sysrootRoot)
	}
	if err := profilesutil.AtomicAction(jirix, sysrootConfigFn, absSysroot, "Configure sysroot"); err != nil {
		return "", nil, err
	}

	vars := []string{
		"CC_FOR_TARGET=" + filepath.Join(ci.ClangBin, "go-"+targetABI+"-clang"),
		"CXX_FOR_TARGET=" + filepath.Join(ci.ClangBin, "go-"+targetABI+"-clang++"),
		"CLANG_BIN=" + filepath.Join(ci.ClangBin),
		"BINUTILS_BIN=" + filepath.Join(ci.BinutilsBin, "bin"),
		"CLANG=" + filepath.Join(ci.ClangBin, "go-"+targetABI+"-clang"),
		"CLANG++=" + filepath.Join(ci.ClangBin, "go-"+targetABI+"-clang++"),
		"LDFLAGS=" + "-lm",
		"AR=" + filepath.Join(ci.BinutilsBin, "ar"),
		"RANLIB=" + filepath.Join(ci.BinutilsBin, "ranlib"),
		"TARGET=" + ci.Target,
	}
	return ci.ClangBin, vars, nil
}

func unpackCommand(file string) (string, []string, error) {
	compressed := false
	compressionOpt := ""
	tar := false
	dirname := filepath.Base(file)
	ext := filepath.Ext(dirname)
	switch ext {
	case ".gz", ".xz":
		compressed = true
		compressionOpt = "z"
		dirname = strings.TrimSuffix(dirname, ext)
	}
	if ext := filepath.Ext(dirname); ext == ".tar" {
		tar = true
		dirname = strings.TrimSuffix(dirname, ".tar")
	}
	if !tar {
		if compressed {
			return file, []string{"unzip", file}, nil
		} else {
			return file, []string{"echo", "it looks like ", file, " is already unpacked and uncompressed"}, nil
		}
	}
	opts := "xf"
	if compressed {
		opts = compressionOpt + opts
	}
	return dirname, []string{"tar", "-" + opts, file}, nil
}

func downloadCmd(uri string) (string, []string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", nil, err
	}
	switch u.Scheme {
	case "gs":
		return path.Base(u.Path), []string{"gsutil", "cp", uri}, nil
	case "http", "https":
		return path.Base(u.Path), []string{"curl", "-LO", uri}, nil
	}
	return "", nil, fmt.Errorf("%v is an unsupported uri:", uri)
}

func downloadAndUnpackFile(jirix *jiri.X, uri, toRoot string) error {
	filename, downloadCmd, err := downloadCmd(uri)
	if err != nil {
		return err
	}
	dirname, unpackCmd, err := unpackCommand(filename)
	if err != nil {
		return err
	}
	toDir := filepath.Join(toRoot, dirname)
	fn := func() error {
		s := jirix.NewSeq()
		tmpDir, err := s.TempDir("", "")
		if err != nil {
			return err
		}
		defer jirix.NewSeq().RemoveAll(tmpDir).Done()
		return s.Pushd(tmpDir).
			Run(downloadCmd[0], downloadCmd[1:]...).
			Run(unpackCmd[0], unpackCmd[1:]...).
			MkdirAll(toRoot, profilesutil.DefaultDirPerm).
			Rename(filepath.Join(tmpDir, dirname), toDir).
			Done()
	}
	return profilesutil.AtomicAction(jirix, fn, toDir, "Download and unpack: "+uri)
}

func rewriteSysroot(jirix *jiri.X, sysroot string) error {
	ch := make(chan *absoluteLink, 100)
	errch := make(chan error, 1)
	go func() {
		findAbsoluteLinks(ch, errch, sysroot)
		close(ch)
	}()
	for {
		select {
		case err := <-errch:
			return err
		case al := <-ch:
			if al == nil {
				return nil
			}
			rewriteLinks(jirix, al.source, al.target, sysroot)
		}
	}
	return nil
}

func rewriteLinks(jirix *jiri.X, pathname, target, sysroot string) error {
	newTarget := filepath.Join(sysroot, target)
	if jirix.DryRun() {
		fmt.Fprintf(jirix.Stdout(), "rewrite: %s -> %q to %q\n", pathname, target, newTarget)
		return nil
	}
	os.Remove(pathname)
	if err := os.Symlink(newTarget, pathname); err != nil {
		return fmt.Errorf("Symlink: %q -> %q: %s\n", pathname, newTarget, err)
	}
	return nil
}

func findAbsoluteLinks(ch chan *absoluteLink, errch chan error, root string) {
	di, err := os.Open(root)
	if err != nil {
		errch <- err
		return
	}
	files, err := di.Readdir(-1)
	if err != nil {
		errch <- err
		return
	}
	di.Close()
	for _, file := range files {
		pathname := filepath.Join(root, file.Name())
		if file.IsDir() {
			findAbsoluteLinks(ch, errch, pathname)
			continue
		}
		if (file.Mode() & os.ModeSymlink) != 0 {
			target, err := os.Readlink(pathname)
			if err != nil {
				errch <- err
				return
			}
			if filepath.IsAbs(target) && !filepath.HasPrefix(target, root) {
				ch <- &absoluteLink{pathname, target}
			}
		}
	}
}
