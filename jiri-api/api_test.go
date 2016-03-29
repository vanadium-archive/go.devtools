// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"v.io/jiri"
	"v.io/jiri/gitutil"
	"v.io/jiri/jiritest"
	"v.io/jiri/project"
	"v.io/jiri/runutil"
	"v.io/jiri/tool"
	"v.io/x/devtools/tooldata"
)

func writeFileOrDie(t *testing.T, jirix *jiri.X, path, contents string) {
	if err := jirix.NewSeq().WriteFile(path, []byte(contents), 0644).Done(); err != nil {
		t.Fatalf("WriteFile(%v, %v) failed: %v", path, contents, err)
	}
}

// setupAPITest sets up the test environment and returns a FakeJiriRoot
// representing the environment that was created, along with a cleanup closure
// that should be deferred.
func setupAPITest(t *testing.T) (*jiritest.FakeJiriRoot, func()) {
	// Capture JIRI_ROOT, using a relative path.  We use this to find the
	// third_party repository below.
	realRoot, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	// Set up a fake jiri environment, with a test project.
	fake, cleanupFake := jiritest.NewFakeJiriRoot(t)
	if err := fake.CreateRemoteProject("test"); err != nil {
		t.Fatal(err)
	}

	if err := fake.AddProject(project.Project{
		Name:   "test",
		Path:   "test",
		Remote: fake.Projects["test"],
	}); err != nil {
		t.Fatal(err)
	}

	// Set up a third_party project, based on the real root.  We need the real
	// third_party sources in order for buildGotools to work.
	if err := fake.CreateRemoteProject("third_party"); err != nil {
		t.Fatal(err)
	}
	if err := fake.AddProject(project.Project{
		Name:   "third_party",
		Path:   "third_party",
		Remote: filepath.Join(realRoot, "third_party"),
	}); err != nil {
		t.Fatal(err)
	}

	if err := fake.UpdateUniverse(false); err != nil {
		t.Fatal(err)
	}

	// Build gotools for use in the rest of the api tests.
	gotoolsPath, cleanupGotools, err := buildGotools(fake.X)
	if err != nil {
		t.Fatalf("buildGotools failed: %v", err)
	}
	gotoolsBinPathFlag = gotoolsPath

	return fake, func() {
		if err := cleanupGotools(); err != nil {
			t.Fatal(err)
		}
		cleanupFake()
		gotoolsBinPathFlag = ""
	}
}

// TestPublicAPICheckError checks that the public API check fails for
// a CL that introduces changes to the public API.
func TestPublicAPICheckError(t *testing.T) {
	fake, cleanup := setupAPITest(t)
	defer cleanup()

	config := tooldata.NewConfig(tooldata.APICheckProjectsOpt(map[string]struct{}{"test": struct{}{}}))
	if err := tooldata.SaveConfig(fake.X, config); err != nil {
		t.Fatalf("%v", err)
	}
	branch := "my-branch"
	projectPath := filepath.Join(fake.X.Root, "test")
	if err := gitutil.New(fake.X.NewSeq(), gitutil.RootDirOpt(projectPath)).CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}

	// Simulate an API with an existing public function called TestFunction.
	writeFileOrDie(t, fake.X, filepath.Join(projectPath, ".api"), `# This is a comment that should be ignored
pkg main, func TestFunction()
`)

	// Write a change that un-exports TestFunction.
	writeFileOrDie(t, fake.X, filepath.Join(projectPath, "file.go"), `package main

func testFunction() {
}`)

	commitMessage := "Commit file.go"
	if err := gitutil.New(fake.X.NewSeq(), gitutil.RootDirOpt(projectPath)).CommitFile("file.go", commitMessage); err != nil {
		t.Fatalf("%v", err)
	}

	var buf bytes.Buffer
	fake.X.Context = tool.NewContext(tool.ContextOpts{Stdout: &buf})
	if err := doAPICheck(fake.X, []string{"test"}, true); err != nil {
		t.Fatalf("doAPICheck failed: %v", err)
	} else if buf.String() == "" {
		t.Fatalf("doAPICheck detected no changes, but some were expected")
	}
}

// TestPublicAPICheckOk checks that the public API check succeeds for
// a CL that introduces no changes to the public API.
func TestPublicAPICheckOk(t *testing.T) {
	fake, cleanup := setupAPITest(t)
	defer cleanup()
	config := tooldata.NewConfig(tooldata.APICheckProjectsOpt(map[string]struct{}{"test": struct{}{}}))
	if err := tooldata.SaveConfig(fake.X, config); err != nil {
		t.Fatalf("%v", err)
	}
	branch := "my-branch"
	projectPath := filepath.Join(fake.X.Root, "test")
	if err := gitutil.New(fake.X.NewSeq(), gitutil.RootDirOpt(projectPath)).CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}

	// Simulate an API with an existing public function called TestFunction.
	writeFileOrDie(t, fake.X, filepath.Join(projectPath, ".api"), `# This is a comment that should be ignored
pkg main, func TestFunction()
`)

	// Write a change that un-exports TestFunction.
	writeFileOrDie(t, fake.X, filepath.Join(projectPath, "file.go"), `package main

func TestFunction() {
}`)

	commitMessage := "Commit file.go"
	if err := gitutil.New(fake.X.NewSeq(), gitutil.RootDirOpt(projectPath)).CommitFile("file.go", commitMessage); err != nil {
		t.Fatalf("%v", err)
	}

	var buf bytes.Buffer
	fake.X.Context = tool.NewContext(tool.ContextOpts{Stdout: &buf})
	if err := doAPICheck(fake.X, []string{"test"}, true); err != nil {
		t.Fatalf("doAPICheck failed: %v", err)
	} else if buf.String() != "" {
		t.Fatalf("doAPICheck detected changes, but none were expected: %s", buf.String())
	}
}

// TestPublicAPIMissingAPIFile ensures that the check will fail if a 'required
// check' project has a missing .api file and a non-empty public API.
func TestPublicAPIMissingAPIFile(t *testing.T) {
	fake, cleanup := setupAPITest(t)
	defer cleanup()
	config := tooldata.NewConfig(tooldata.APICheckProjectsOpt(map[string]struct{}{"test": struct{}{}}))
	if err := tooldata.SaveConfig(fake.X, config); err != nil {
		t.Fatalf("%v", err)
	}
	branch := "my-branch"
	projectPath := filepath.Join(fake.X.Root, "test")
	if err := gitutil.New(fake.X.NewSeq(), gitutil.RootDirOpt(projectPath)).CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}

	// Write a go file with a public API and no corresponding .api file.
	writeFileOrDie(t, fake.X, filepath.Join(projectPath, "file.go"), `package main

func TestFunction() {
}`)

	commitMessage := "Commit file.go"
	if err := gitutil.New(fake.X.NewSeq(), gitutil.RootDirOpt(projectPath)).CommitFile("file.go", commitMessage); err != nil {
		t.Fatalf("%v", err)
	}

	var buf bytes.Buffer
	fake.X.Context = tool.NewContext(tool.ContextOpts{Stdout: &buf})
	if err := doAPICheck(fake.X, []string{"test"}, true); err != nil {
		t.Fatalf("doAPICheck failed: %v", err)
	} else if buf.String() == "" {
		t.Fatalf("doAPICheck should have failed, but did not")
	} else if !strings.Contains(buf.String(), "could not read the package's .api file") {
		t.Fatalf("doAPICheck failed, but not for the expected reason: %s", buf.String())
	}
}

// TestPublicAPIMissingAPIFileNoPublicAPI ensures that the check will pass if a
// 'required check' project has a missing .api but the public API is empty.
func TestPublicAPIMissingAPIFileNoPublicAPI(t *testing.T) {
	fake, cleanup := setupAPITest(t)
	defer cleanup()
	config := tooldata.NewConfig(tooldata.APICheckProjectsOpt(map[string]struct{}{"test": struct{}{}}))
	if err := tooldata.SaveConfig(fake.X, config); err != nil {
		t.Fatalf("%v", err)
	}
	branch := "my-branch"
	projectPath := filepath.Join(fake.X.Root, "test")
	if err := gitutil.New(fake.X.NewSeq(), gitutil.RootDirOpt(projectPath)).CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}

	// Write a go file with a public API and no corresponding .api file.
	writeFileOrDie(t, fake.X, filepath.Join(projectPath, "file.go"), `package main

func testFunction() {
}`)

	commitMessage := "Commit file.go"
	if err := gitutil.New(fake.X.NewSeq(), gitutil.RootDirOpt(projectPath)).CommitFile("file.go", commitMessage); err != nil {
		t.Fatalf("%v", err)
	}

	var buf bytes.Buffer
	fake.X.Context = tool.NewContext(tool.ContextOpts{Stdout: &buf})
	if err := doAPICheck(fake.X, []string{"test"}, true); err != nil {
		t.Fatalf("doAPICheck failed: %v", err)
	} else if output := buf.String(); output != "" {
		t.Fatalf("doAPICheck should have passed, but did not: %s", output)
	}
}

// TestPublicAPIMissingAPIFileNotRequired ensures that the check will
// not fail if a 'required check' project has a missing .api file but
// that API file is in an 'internal' package.
func TestPublicAPIMissingAPIFileNotRequired(t *testing.T) {
	fake, cleanup := setupAPITest(t)
	defer cleanup()
	config := tooldata.NewConfig(tooldata.APICheckProjectsOpt(map[string]struct{}{"test": struct{}{}}))
	if err := tooldata.SaveConfig(fake.X, config); err != nil {
		t.Fatalf("%v", err)
	}
	branch := "my-branch"
	projectPath := filepath.Join(fake.X.Root, "test")
	if err := gitutil.New(fake.X.NewSeq(), gitutil.RootDirOpt(projectPath)).CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}

	// Write a go file with a public API and no corresponding .api file.
	if err := os.Mkdir(filepath.Join(projectPath, "internal"), 0744); err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}
	testFilePath := filepath.Join(projectPath, "internal", "file.go")
	writeFileOrDie(t, fake.X, testFilePath, `package main

func TestFunction() {
}`)

	commitMessage := "Commit file.go"
	if err := gitutil.New(fake.X.NewSeq(), gitutil.RootDirOpt(projectPath)).CommitFile(testFilePath, commitMessage); err != nil {
		t.Fatalf("%v", err)
	}

	var buf bytes.Buffer
	fake.X.Context = tool.NewContext(tool.ContextOpts{Stdout: &buf})
	if err := doAPICheck(fake.X, []string{"test"}, true); err != nil {
		t.Fatalf("doAPICheck failed: %v", err)
	} else if buf.String() != "" {
		t.Fatalf("doAPICheck should have passed, but did not: %s", buf.String())
	}
}

// TestPublicAPIUpdate checks that the api update command correctly
// updates the API definition.
func TestPublicAPIUpdate(t *testing.T) {
	fake, cleanup := setupAPITest(t)
	defer cleanup()
	if err := tooldata.SaveConfig(fake.X, tooldata.NewConfig()); err != nil {
		t.Fatalf("%v", err)
	}
	branch := "my-branch"
	projectPath := filepath.Join(fake.X.Root, "test")
	if err := gitutil.New(fake.X.NewSeq(), gitutil.RootDirOpt(projectPath)).CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}

	// Simulate an API with an existing public function called TestFunction.
	writeFileOrDie(t, fake.X, filepath.Join(projectPath, ".api"), `# This is a comment that should be ignored
pkg main, func TestFunction()
`)

	// Write a change that changes TestFunction to TestFunction1.
	writeFileOrDie(t, fake.X, filepath.Join(projectPath, "file.go"), `package main

func TestFunction1() {
}`)

	commitMessage := "Commit file.go"
	if err := gitutil.New(fake.X.NewSeq(), gitutil.RootDirOpt(projectPath)).CommitFile("file.go", commitMessage); err != nil {
		t.Fatalf("%v", err)
	}

	var out bytes.Buffer
	fake.X.Context = tool.NewContext(tool.ContextOpts{Stdout: &out, Stderr: &out})
	if err := runAPIFix(fake.X, []string{"test"}); err != nil {
		t.Fatalf("should have succeeded but did not: %v\n%v", err, out.String())
	}

	contents, err := readAPIFileContents(fake.X, filepath.Join(projectPath, ".api"))
	if err != nil {
		t.Fatalf("%v", err)
	}

	if got, want := string(contents), "pkg main, func TestFunction1()\n"; got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}

	// Now write a change that changes TestFunction1 to testFunction1.
	// There should be no more public API left and the .api file should be
	// removed.
	writeFileOrDie(t, fake.X, filepath.Join(projectPath, "file.go"), `package main

func testFunction1() {
}`)
	if err := gitutil.New(fake.X.NewSeq(), gitutil.RootDirOpt(projectPath)).CommitFile("file.go", commitMessage); err != nil {
		t.Fatalf("%v", err)
	}

	out.Reset()
	if err := runAPIFix(fake.X, []string{"test"}); err != nil {
		t.Fatalf("should have succeeded but did not: %v", err)
	}
	if _, err := fake.X.NewSeq().Stat(filepath.Join(projectPath, ".api")); err == nil {
		t.Fatalf(".api file exists when it should have been removed: %v", err)
	} else if !runutil.IsNotExist(err) {
		t.Fatalf("%v", err)
	}
}
