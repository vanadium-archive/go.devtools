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

	"v.io/jiri/jiri"
	"v.io/jiri/project"
	"v.io/jiri/tool"
)

func TestCopyright(t *testing.T) {
	var errOut bytes.Buffer
	ctx := tool.NewContext(tool.ContextOpts{
		Stderr: io.MultiWriter(os.Stderr, &errOut),
	})
	jirix := &jiri.X{Context: ctx}

	// Load assets.
	dataDir, err := project.DataDirPath(jirix, "jiri")
	if err != nil {
		t.Fatalf("%v", err)
	}
	assets, err := loadAssets(jirix, dataDir)
	if err != nil {
		t.Fatalf("%v", err)
	}

	// Setup a fake JIRI_ROOT.
	root, err := project.NewFakeJiriRoot()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer func() {
		if err := root.Cleanup(); err != nil {
			t.Fatalf("%v", err)
		}
	}()
	if err := root.CreateRemoteProject("test"); err != nil {
		t.Fatalf("%v", err)
	}
	if err := root.AddProject(project.Project{
		Name:   "test",
		Path:   "test",
		Remote: root.Projects["test"],
	}); err != nil {
		t.Fatalf("%v", err)
	}
	if err := root.UpdateUniverse(false); err != nil {
		t.Fatalf("%v", err)
	}

	oldRoot, err := project.JiriRoot()
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := os.Setenv("JIRI_ROOT", root.Dir); err != nil {
		t.Fatalf("Setenv() failed: %v", err)
	}
	defer os.Setenv("JIRI_ROOT", oldRoot)

	allFiles := map[string]string{}
	for file, data := range assets.MatchFiles {
		allFiles[file] = data
	}
	for file, data := range assets.MatchPrefixFiles {
		allFiles[file] = data
	}

	// Write out test licensing files and sample source code files to a
	// project and verify that the project checks out.
	projectPath := filepath.Join(root.Dir, "test")
	project := project.Project{Path: projectPath}
	for _, lang := range languages {
		file := "test" + lang.FileExtension
		if err := jirix.Run().WriteFile(filepath.Join(projectPath, file), nil, os.FileMode(0600)); err != nil {
			t.Fatalf("%v", err)
		}
		missing, err := checkFile(jirix, filepath.Join(project.Path, file), assets, true)
		if err != nil {
			t.Fatalf("%v", err)
		}
		if got, want := missing, false; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		if err := jirix.Git(tool.RootDirOpt(projectPath)).CommitFile(file, "adding "+file); err != nil {
			t.Fatalf("%v", err)
		}
	}
	for file, data := range allFiles {
		if err := jirix.Run().WriteFile(filepath.Join(projectPath, file), []byte(data), os.FileMode(0600)); err != nil {
			t.Fatalf("%v", err)
		}
		if err := jirix.Git(tool.RootDirOpt(projectPath)).CommitFile(file, "adding "+file); err != nil {
			t.Fatalf("%v", err)
		}
	}
	missing, err := checkProject(jirix, project, assets, false)
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
		missing, err := checkProject(jirix, project, assets, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, false; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		path := filepath.Join(projectPath, file)
		if err := jirix.Git(tool.RootDirOpt(projectPath)).Remove(file); err != nil {
			t.Fatalf("%v", err)
		}
		missing, err = checkProject(jirix, project, assets, false)
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
		missing, err := checkProject(jirix, project, assets, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, false; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		path := filepath.Join(projectPath, file)
		if err := jirix.Run().WriteFile(path, []byte("garbage"), os.FileMode(0600)); err != nil {
			t.Fatalf("%v", err)
		}
		missing, err = checkProject(jirix, project, assets, false)
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
		missing, err := checkProject(jirix, project, assets, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, false; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		path := filepath.Join(projectPath, "test"+lang.FileExtension)
		if err := jirix.Run().WriteFile(path, []byte("garbage"), os.FileMode(0600)); err != nil {
			t.Fatalf("%v", err)
		}
		missing, err = checkProject(jirix, project, assets, false)
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
		missing, err := checkProject(jirix, project, assets, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, false; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		path := filepath.Join(projectPath, file)
		if err := jirix.Run().RemoveAll(path); err != nil {
			t.Fatalf("%v", err)
		}
		missing, err = checkProject(jirix, project, assets, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, false; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		missing, err = checkProject(jirix, project, assets, false)
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
		missing, err := checkProject(jirix, project, assets, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, false; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		path := filepath.Join(projectPath, file)
		if err := jirix.Run().WriteFile(path, []byte("garbage"), os.FileMode(0600)); err != nil {
			t.Fatalf("%v", err)
		}
		missing, err = checkProject(jirix, project, assets, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, false; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		missing, err = checkProject(jirix, project, assets, false)
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
		missing, err := checkProject(jirix, project, assets, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, false; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		path := filepath.Join(projectPath, "test"+lang.FileExtension)
		if err := jirix.Run().WriteFile(path, []byte("garbage"), os.FileMode(0600)); err != nil {
			t.Fatalf("%v", err)
		}
		missing, err = checkProject(jirix, project, assets, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := missing, false; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
		missing, err = checkProject(jirix, project, assets, false)
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
	missing, err = checkProject(jirix, project, assets, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := missing, false; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	path := filepath.Join(projectPath, "third_party")
	if err := jirix.Run().MkdirAll(path, 0700); err != nil {
		t.Fatalf("%v", err)
	}
	path = filepath.Join(path, "test.go")
	if err := jirix.Run().WriteFile(path, []byte("garbage"), os.FileMode(0600)); err != nil {
		t.Fatalf("%v", err)
	}
	// Since this file is in a subdir, we must run "git add" to have git track it.
	// Without this, the test passes regardless of the subdir name.
	if err := jirix.Git(tool.RootDirOpt(projectPath)).Add(path); err != nil {
		t.Fatalf("%v", err)
	}
	missing, err = checkProject(jirix, project, assets, false)
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
	if err := jirix.Run().WriteFile(ignoreFile, []byte("public/fancy.js"), os.FileMode(0600)); err != nil {
		t.Fatalf("%v", err)
	}
	publicDir := filepath.Join(projectPath, "public")
	if err := jirix.Run().MkdirAll(publicDir, 0700); err != nil {
		t.Fatalf("%v", err)
	}
	filename := filepath.Join(publicDir, "fancy.js")
	if err := jirix.Run().WriteFile(filename, []byte("garbage"), os.FileMode(0600)); err != nil {
		t.Fatalf("%v", err)
	}
	// Since the copyright check only applies to tracked files, we must run "git
	// add" to have git track it. Without this, the test passes regardless of the
	// subdir name.
	if err := jirix.Git(tool.RootDirOpt(projectPath)).Add(filename); err != nil {
		t.Fatalf("%v", err)
	}
	missing, err = checkProject(jirix, project, assets, false)
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
		if ignore := isIgnored(path, expressions); !ignore {
			t.Errorf("isIgnored(%s, expressions) == %v, should be %v", path, ignore, true)
		}
	}

	shouldNotIgnore := []string{"foo", "bar"}
	for _, path := range shouldNotIgnore {
		if ignore := isIgnored(path, expressions); ignore {
			t.Errorf("isIgnored(%s, expressions) == %v, should be %v", path, ignore, false)
		}
	}
}
