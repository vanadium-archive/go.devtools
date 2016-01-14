// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"v.io/jiri/gitutil"
	"v.io/jiri/jiritest"
	"v.io/jiri/project"
	"v.io/jiri/tool"
)

func TestCopyright(t *testing.T) {
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()

	var errOut bytes.Buffer
	fake.X.Context = tool.NewContext(tool.ContextOpts{
		Stderr: io.MultiWriter(os.Stderr, &errOut),
	})

	// Load assets.
	assets, err := loadAssets(fake.X, filepath.Join("..", "data"))
	if err != nil {
		t.Fatalf("%v", err)
	}

	if err := fake.CreateRemoteProject("test"); err != nil {
		t.Fatalf("%v", err)
	}
	if err := fake.AddProject(project.Project{
		Name:   "test",
		Path:   "test",
		Remote: fake.Projects["test"],
	}); err != nil {
		t.Fatalf("%v", err)
	}
	if err := fake.UpdateUniverse(false); err != nil {
		t.Fatalf("%v", err)
	}

	allFiles := map[string]string{}
	for file, data := range assets.MatchFiles {
		allFiles[file] = data
	}
	for file, data := range assets.MatchPrefixFiles {
		allFiles[file] = data
	}

	// Write out test licensing files and sample source code files to a
	// project and verify that the project checks out.
	projectPath := filepath.Join(fake.X.Root, "test")
	project := project.Project{Path: projectPath}
	s := fake.X.NewSeq()
	for _, lang := range languages {
		file := "test" + lang.FileExtension
		if err := s.WriteFile(filepath.Join(projectPath, file), nil, os.FileMode(0600)).Done(); err != nil {
			t.Fatalf("%v", err)
		}
		missing, err := checkFile(fake.X, filepath.Join(project.Path, file), assets, true)
		if err != nil {
			t.Fatalf("%v", err)
		}
		if got, want := missing, false; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		if err := gitutil.New(fake.X.NewSeq(), gitutil.RootDirOpt(projectPath)).CommitFile(file, "adding "+file); err != nil {
			t.Fatalf("%v", err)
		}
	}
	for file, data := range allFiles {
		if err := s.WriteFile(filepath.Join(projectPath, file), []byte(data), os.FileMode(0600)).Done(); err != nil {
			t.Fatalf("%v", err)
		}
		if err := gitutil.New(fake.X.NewSeq(), gitutil.RootDirOpt(projectPath)).CommitFile(file, "adding "+file); err != nil {
			t.Fatalf("%v", err)
		}
	}
	missing, err := checkProject(fake.X, project, assets, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := missing, false; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := errOut.String(), ""; got != want {
		t.Fatalf("unexpected error message: got %q, want %q", got, want)
	}
	// Check that missing licensing files are reported correctly.
	for file, _ := range allFiles {
		errOut.Reset()
		missing, err := checkProject(fake.X, project, assets, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, false; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		path := filepath.Join(projectPath, file)
		if err := gitutil.New(fake.X.NewSeq(), gitutil.RootDirOpt(projectPath)).Remove(file); err != nil {
			t.Fatalf("%v", err)
		}
		missing, err = checkProject(fake.X, project, assets, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, true; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		if got, want := errOut.String(), fmt.Sprintf("%v is missing\n", path); got != want {
			t.Fatalf("unexpected error message: got %q, want %q", got, want)
		}
	}
	// Check that out-of-date licensing files are reported correctly.
	for file, _ := range allFiles {
		errOut.Reset()
		missing, err := checkProject(fake.X, project, assets, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, false; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		path := filepath.Join(projectPath, file)
		if err := s.WriteFile(path, []byte("garbage"), os.FileMode(0600)).Done(); err != nil {
			t.Fatalf("%v", err)
		}
		missing, err = checkProject(fake.X, project, assets, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, true; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		if got, want := errOut.String(), fmt.Sprintf("%v is not up-to-date\n", path); got != want {
			t.Fatalf("unexpected error message: got %q, want %q", got, want)
		}
	}

	// Check that source code files without the copyright header are
	// reported correctly.
	for _, lang := range languages {
		errOut.Reset()
		missing, err := checkProject(fake.X, project, assets, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, false; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		path := filepath.Join(projectPath, "test"+lang.FileExtension)
		if err := s.WriteFile(path, []byte("garbage"), os.FileMode(0600)).Done(); err != nil {
			t.Fatalf("%v", err)
		}
		missing, err = checkProject(fake.X, project, assets, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, true; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		if got, want := errOut.String(), fmt.Sprintf("%v copyright is missing\n", path); got != want {
			t.Fatalf("unexpected error message: got %q, want %q", got, want)
		}
	}

	// Check that missing licensing files are fixed up correctly.
	for file, _ := range allFiles {
		errOut.Reset()
		missing, err := checkProject(fake.X, project, assets, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, false; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		path := filepath.Join(projectPath, file)
		if err := s.RemoveAll(path).Done(); err != nil {
			t.Fatalf("%v", err)
		}
		missing, err = checkProject(fake.X, project, assets, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, false; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		missing, err = checkProject(fake.X, project, assets, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, false; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		if got, want := errOut.String(), ""; got != want {
			t.Fatalf("unexpected error message: got %q, want %q", got, want)
		}
	}

	// Check that out-of-date licensing files are fixed up correctly.
	for file, _ := range allFiles {
		errOut.Reset()
		missing, err := checkProject(fake.X, project, assets, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, false; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		path := filepath.Join(projectPath, file)
		if err := s.WriteFile(path, []byte("garbage"), os.FileMode(0600)).Done(); err != nil {
			t.Fatalf("%v", err)
		}
		missing, err = checkProject(fake.X, project, assets, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, false; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		missing, err = checkProject(fake.X, project, assets, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, false; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		if got, want := errOut.String(), ""; got != want {
			t.Fatalf("unexpected error message: got %q, want %q", got, want)
		}
	}

	// Check that source code files missing the copyright header are
	// fixed up correctly.
	for _, lang := range languages {
		errOut.Reset()
		missing, err := checkProject(fake.X, project, assets, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, false; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		path := filepath.Join(projectPath, "test"+lang.FileExtension)
		if err := s.WriteFile(path, []byte("garbage"), os.FileMode(0600)).Done(); err != nil {
			t.Fatalf("%v", err)
		}
		missing, err = checkProject(fake.X, project, assets, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, false; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		missing, err = checkProject(fake.X, project, assets, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, false; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		if got, want := errOut.String(), ""; got != want {
			t.Fatalf("unexpected error message: got %q, want %q", got, want)
		}
	}

	// Check that third-party files are skipped when checking for copyright
	// headers.
	errOut.Reset()
	missing, err = checkProject(fake.X, project, assets, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := missing, false; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	path := filepath.Join(projectPath, "third_party")
	gpath := filepath.Join(path, "test.go")
	if err := s.MkdirAll(path, 0700).
		WriteFile(gpath, []byte("garbage"), os.FileMode(0600)).Done(); err != nil {
		t.Fatalf("%v", err)
	}
	// Since this file is in a subdir, we must run "git add" to have git track it.
	// Without this, the test passes regardless of the subdir name.
	if err := gitutil.New(fake.X.NewSeq(), gitutil.RootDirOpt(projectPath)).Add(path); err != nil {
		t.Fatalf("%v", err)
	}
	missing, err = checkProject(fake.X, project, assets, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := missing, false; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if errOut.Len() != 0 {
		t.Fatalf("unexpected error message: %q", errOut.String())
	}

	// Test .jiriignore functionality.
	errOut.Reset()
	// Add .jiriignore file.
	ignoreFile := filepath.Join(projectPath, jiriIgnore)
	publicDir := filepath.Join(projectPath, "public")
	filename := filepath.Join(publicDir, "fancy.js")

	if err := s.
		WriteFile(ignoreFile, []byte("public/fancy.js"), os.FileMode(0600)).
		MkdirAll(publicDir, 0700).
		WriteFile(filename, []byte("garbage"), os.FileMode(0600)).Done(); err != nil {
		t.Fatalf("%v", err)
	}

	// Since the copyright check only applies to tracked files, we must run "git
	// add" to have git track it. Without this, the test passes regardless of the
	// subdir name.
	if err := gitutil.New(fake.X.NewSeq(), gitutil.RootDirOpt(projectPath)).Add(filename); err != nil {
		t.Fatalf("%v", err)
	}
	missing, err = checkProject(fake.X, project, assets, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := missing, false; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if errOut.Len() != 0 {
		t.Fatalf("unexpected error message: %q", errOut.String())
	}
}

func TestCopyrightIsIgnored(t *testing.T) {
	lines := []string{
		"public/bundle.*",
		"build/",
		"dist/min.js",
	}
	expressions := []*regexp.Regexp{}
	for _, line := range lines {
		expression, err := regexp.Compile(line)
		if err != nil {
			t.Fatalf("unexpected regexp.Compile(%v) failed: %v", line, err)
		}

		expressions = append(expressions, expression)
	}

	shouldIgnore := []string{
		"public/bundle.js",
		"public/bundle.css",
		"dist/min.js",
		"build/bar",
	}

	for _, path := range shouldIgnore {
		if ignore, _ := isIgnored(path, expressions); !ignore {
			t.Errorf("isIgnored(%s, expressions) == %v, should be %v", path, ignore, true)
		}
	}

	shouldNotIgnore := []string{"foo", "bar"}
	for _, path := range shouldNotIgnore {
		if ignore, _ := isIgnored(path, expressions); ignore {
			t.Errorf("isIgnored(%s, expressions) == %v, should be %v", path, ignore, false)
		}
	}
}
