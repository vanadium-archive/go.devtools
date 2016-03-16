// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tooldata_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"v.io/jiri/jiritest"
	"v.io/jiri/project"
	"v.io/x/devtools/tooldata"
)

var (
	apiCheckProjects = map[string]struct{}{
		"projectA": struct{}{},
		"projectB": struct{}{},
	}
	copyrightCheckProjects = map[string]struct{}{
		"projectC": struct{}{},
		"projectD": struct{}{},
	}
	goWorkspaces      = []string{"test-go-workspace"}
	jenkinsMatrixJobs = map[string]tooldata.JenkinsMatrixJobInfo{
		"test-job-A": {
			HasArch:  false,
			HasOS:    true,
			HasParts: true,
			ShowOS:   false,
			Name:     "test-job-A",
		},
		"test-job-B": {
			HasArch:  true,
			HasOS:    false,
			HasParts: false,
			ShowOS:   false,
			Name:     "test-job-B",
		},
	}
	projectTests = map[string][]string{
		"test-project":  []string{"test-test-A", "test-test-group"},
		"test-project2": []string{"test-test-D"},
	}
	testDependencies = map[string][]string{
		"test-test-A": []string{"test-test-B"},
		"test-test-B": []string{"test-test-C"},
	}
	testGroups = map[string][]string{
		"test-test-group": []string{"test-test-B", "test-test-C"},
	}
	testParts = map[string][]string{
		"test-test-A": []string{"p1", "p2"},
	}
	vdlWorkspaces = []string{"test-vdl-workspace"}
)

func testConfigAPI(t *testing.T, c *tooldata.Config) {
	if got, want := c.APICheckProjects(), apiCheckProjects; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected results: got %v, want %v", got, want)
	}
	if got, want := c.CopyrightCheckProjects(), copyrightCheckProjects; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected results: got %v, want %v", got, want)
	}
	if got, want := c.GoWorkspaces(), goWorkspaces; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected result: got %v, want %v", got, want)
	}
	if got, want := c.GroupTests([]string{"test-test-group"}), []string{"test-test-B", "test-test-C"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected result: got %v, want %v", got, want)
	}
	if got, want := c.JenkinsMatrixJobs(), jenkinsMatrixJobs; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected result: got %v, want %v", got, want)
	}
	if got, want := c.Projects(), []string{"test-project", "test-project2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected result: got %v, want %v", got, want)
	}
	if got, want := c.ProjectTests([]string{"test-project"}), []string{"test-test-A", "test-test-B", "test-test-C"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected result: got %v, want %v", got, want)
	}
	if got, want := c.ProjectTests([]string{"test-project", "test-project2"}), []string{"test-test-A", "test-test-B", "test-test-C", "test-test-D"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected result: got %v, want %v", got, want)
	}
	if got, want := c.TestDependencies("test-test-A"), []string{"test-test-B"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected result: got %v, want %v", got, want)
	}
	if got, want := c.TestDependencies("test-test-B"), []string{"test-test-C"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected result: got %v, want %v", got, want)
	}
	if got, want := c.TestParts("test-test-A"), []string{"p1", "p2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected result: got %v, want %v", got, want)
	}
	if got, want := c.VDLWorkspaces(), vdlWorkspaces; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected result: got %v, want %v", got, want)
	}
}

func TestConfigAPI(t *testing.T) {
	config := tooldata.NewConfig(
		tooldata.APICheckProjectsOpt(apiCheckProjects),
		tooldata.CopyrightCheckProjectsOpt(copyrightCheckProjects),
		tooldata.GoWorkspacesOpt(goWorkspaces),
		tooldata.JenkinsMatrixJobsOpt(jenkinsMatrixJobs),
		tooldata.ProjectTestsOpt(projectTests),
		tooldata.TestDependenciesOpt(testDependencies),
		tooldata.TestGroupsOpt(testGroups),
		tooldata.TestPartsOpt(testParts),
		tooldata.VDLWorkspacesOpt(vdlWorkspaces),
	)

	testConfigAPI(t, config)
}

func TestConfigSerialization(t *testing.T) {
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()

	config := tooldata.NewConfig(
		tooldata.APICheckProjectsOpt(apiCheckProjects),
		tooldata.CopyrightCheckProjectsOpt(copyrightCheckProjects),
		tooldata.GoWorkspacesOpt(goWorkspaces),
		tooldata.JenkinsMatrixJobsOpt(jenkinsMatrixJobs),
		tooldata.ProjectTestsOpt(projectTests),
		tooldata.TestDependenciesOpt(testDependencies),
		tooldata.TestGroupsOpt(testGroups),
		tooldata.TestPartsOpt(testParts),
		tooldata.VDLWorkspacesOpt(vdlWorkspaces),
	)

	if err := tooldata.SaveConfig(fake.X, config); err != nil {
		t.Fatalf("%v", err)
	}
	gotConfig, err := tooldata.LoadConfig(fake.X)
	if err != nil {
		t.Fatalf("%v", err)
	}

	testConfigAPI(t, gotConfig)
}

func testSetPathHelper(t *testing.T, name string) {
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()

	// Create a test project and identify it as a Go workspace.
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
	var config *tooldata.Config
	switch name {
	case "GOPATH":
		config = tooldata.NewConfig(tooldata.GoWorkspacesOpt([]string{"test", "does/not/exist"}))
	case "VDLPATH":
		config = tooldata.NewConfig(tooldata.VDLWorkspacesOpt([]string{"test", "does/not/exist"}))
	}

	if err := tooldata.SaveConfig(fake.X, config); err != nil {
		t.Fatalf("%v", err)
	}

	var got, want string
	switch name {
	case "GOPATH":
		want = "GOPATH=" + filepath.Join(fake.X.Root, "test")
		got = config.GoPath(fake.X)
	case "VDLPATH":
		// Make a fake src directory.
		want = filepath.Join(fake.X.Root, "test", "src")
		if err := fake.X.NewSeq().MkdirAll(want, 0755).Done(); err != nil {
			t.Fatalf("%v", err)
		}
		want = "VDLPATH=" + want
		got = config.VDLPath(fake.X)
	}
	if got != want {
		t.Fatalf("unexpected value: got %v, want %v", got, want)
	}
}

func TestGoPath(t *testing.T) {
	testSetPathHelper(t, "GOPATH")
}

func TestVDLPath(t *testing.T) {
	testSetPathHelper(t, "VDLPATH")
}
