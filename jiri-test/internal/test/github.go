// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"fmt"
	"os"
	"path/filepath"

	"v.io/jiri"
	"v.io/jiri/collect"
	"v.io/jiri/gitutil"
	"v.io/jiri/runutil"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/xunit"
)

var (
	mirrors = []Mirror{
		Mirror{
			name:         "baku",
			googlesource: "https://vanadium.googlesource.com/release.projects.baku",
			github:       "git@github.com:vanadium/baku.git",
		},
		Mirror{
			name:         "browser",
			googlesource: "https://vanadium.googlesource.com/release.projects.browser",
			github:       "git@github.com:vanadium/browser.git",
		},
		Mirror{
			name:         "chat",
			googlesource: "https://vanadium.googlesource.com/release.projects.chat",
			github:       "git@github.com:vanadium/chat.git",
		},
		Mirror{
			name:         "croupier",
			googlesource: "https://vanadium.googlesource.com/release.projects.croupier",
			github:       "git@github.com:vanadium/croupier.git",
		},
		Mirror{
			name:         "go.devtools",
			googlesource: "https://vanadium.googlesource.com/release.go.x.devtools",
			github:       "git@github.com:vanadium/go.devtools.git",
		},
		Mirror{
			name:         "go.jiri",
			googlesource: "https://vanadium.googlesource.com/release.go.jiri",
			github:       "git@github.com:vanadium/go.jiri.git",
		},
		Mirror{
			name:         "go.jni",
			googlesource: "https://vanadium.googlesource.com/release.go.x.jni",
			github:       "git@github.com:vanadium/go.jni.git",
		},
		Mirror{
			name:         "go.lib",
			googlesource: "https://vanadium.googlesource.com/release.go.x.lib",
			github:       "git@github.com:vanadium/go.lib.git",
		},
		Mirror{
			name:         "go.ref",
			googlesource: "https://vanadium.googlesource.com/release.go.x.ref",
			github:       "git@github.com:vanadium/go.ref.git",
		},
		Mirror{
			name:         "go.swift",
			googlesource: "https://vanadium.googlesource.com/release.go.x.swift",
			github:       "git@github.com:vanadium/go.swift.git",
		},
		Mirror{
			name:         "go.v23",
			googlesource: "https://vanadium.googlesource.com/release.go.v23",
			github:       "git@github.com:vanadium/go.v23.git",
		},
		Mirror{
			name:         "java",
			googlesource: "https://vanadium.googlesource.com/release.java",
			github:       "git@github.com:vanadium/java.git",
		},
		Mirror{
			name:         "js",
			googlesource: "https://vanadium.googlesource.com/release.js.core",
			github:       "git@github.com:vanadium/js.git",
		},
		Mirror{
			name:         "js.syncbase",
			googlesource: "https://vanadium.googlesource.com/release.js.syncbase",
			github:       "git@github.com:vanadium/js.syncbase.git",
		},
		Mirror{
			name:         "luma.third_party",
			googlesource: "https://vanadium.googlesource.com/release.projects.luma_third_party",
			github:       "git@github.com:vanadium/luma.third_party.git",
		},
		Mirror{
			name:         "madb",
			googlesource: "https://vanadium.googlesource.com/release.projects.madb",
			github:       "git@github.com:vanadium/madb.git",
			followTags:   true,
		},
		Mirror{
			name:         "manifest",
			googlesource: "https://vanadium.googlesource.com/manifest",
			github:       "git@github.com:vanadium/manifest.git",
		},
		Mirror{
			name:         "media-sharing",
			googlesource: "https://vanadium.googlesource.com/release.projects.media-sharing",
			github:       "git@github.com:vanadium/media-sharing.git",
		},
		Mirror{
			name:         "mojo.discovery",
			googlesource: "https://vanadium.googlesource.com/release.mojo.discovery",
			github:       "git@github.com:vanadium/mojo.discovery.git",
		},
		Mirror{
			name:         "mojo.shared",
			googlesource: "https://vanadium.googlesource.com/release.mojo.shared",
			github:       "git@github.com:vanadium/mojo.shared.git",
		},
		Mirror{
			name:         "mojo.syncbase",
			googlesource: "https://vanadium.googlesource.com/release.mojo.syncbase",
			github:       "git@github.com:vanadium/mojo.syncbase.git",
		},
		Mirror{
			name:         "mojo.v23proxy",
			googlesource: "https://vanadium.googlesource.com/release.mojo.v23proxy",
			github:       "git@github.com:vanadium/mojo.v23proxy.git",
		},
		Mirror{
			name:         "physical-lock",
			googlesource: "https://vanadium.googlesource.com/release.projects.physical-lock",
			github:       "git@github.com:vanadium/physical-lock.git",
		},
		Mirror{
			name:         "pipe2browser",
			googlesource: "https://vanadium.googlesource.com/release.projects.pipe2browser",
			github:       "git@github.com:vanadium/pipe2browser.git",
		},
		Mirror{
			name:         "playground",
			googlesource: "https://vanadium.googlesource.com/release.projects.playground",
			github:       "git@github.com:vanadium/playground.git",
		},
		Mirror{
			name:         "reader",
			googlesource: "https://vanadium.googlesource.com/release.projects.reader",
			github:       "git@github.com:vanadium/reader.git",
		},
		Mirror{
			name:         "swift",
			googlesource: "https://vanadium.googlesource.com/release.swift",
			github:       "git@github.com:vanadium/swift.git",
		},
		Mirror{
			name:         "syncslides",
			googlesource: "https://vanadium.googlesource.com/release.projects.syncslides",
			github:       "git@github.com:vanadium/syncslides.git",
		},
		Mirror{
			name:         "third_party",
			googlesource: "https://vanadium.googlesource.com/third_party",
			github:       "git@github.com:vanadium/third_party.git",
		},
		Mirror{
			name:         "todos",
			googlesource: "https://vanadium.googlesource.com/release.projects.todos",
			github:       "git@github.com:vanadium/todos.git",
		},
		Mirror{
			name:         "travel",
			googlesource: "https://vanadium.googlesource.com/release.projects.travel",
			github:       "git@github.com:vanadium/travel.git",
		},
		Mirror{
			name:         "website",
			googlesource: "https://vanadium.googlesource.com/website",
			github:       "git@github.com:vanadium/website.git",
		},
	}
)

type Mirror struct {
	name, googlesource, github string
	followTags                 bool
}

// vanadiumGitHubMirror mirrors googlesource.com vanadium projects to
// github.com.
func vanadiumGitHubMirror(jirix *jiri.X, testName string, _ ...Opt) (_ *test.Result, e error) {
	// Initialize the test/task.
	cleanup, err := initTest(jirix, testName, nil)
	if err != nil {
		return nil, newInternalError(err, "Init")
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	projects := filepath.Join(jirix.Root, "projects")
	mode := os.FileMode(0755)
	if err := jirix.NewSeq().MkdirAll(projects, mode).Done(); err != nil {
		return nil, newInternalError(err, "MkdirAll")
	}

	allPassed := true
	suites := []xunit.TestSuite{}
	for _, mirror := range mirrors {
		suite, err := gitHubSync(jirix, mirror, projects)
		if err != nil {
			return nil, newInternalError(err, "sync")
		}

		allPassed = allPassed && (suite.Failures == 0)
		suites = append(suites, *suite)
	}

	if err := xunit.CreateReport(jirix, testName, suites); err != nil {
		return nil, err
	}

	if !allPassed {
		return &test.Result{Status: test.Failed}, nil
	}

	return &test.Result{Status: test.Passed}, nil
}

func gitHubSync(jirix *jiri.X, mirror Mirror, projects string) (*xunit.TestSuite, error) {
	suite := xunit.TestSuite{Name: mirror.name}
	dirname := filepath.Join(projects, mirror.name)

	// If dirname does not exist `git clone` otherwise `git fetch` and
	// `git reset --hard origin/master`.
	if _, err := jirix.NewSeq().Stat(dirname); err != nil {
		if !runutil.IsNotExist(err) {
			return nil, newInternalError(err, "stat")
		}

		err := clone(jirix, mirror, projects)
		testCase := makeTestCase("clone", err)
		if err != nil {
			suite.Failures++
		}
		suite.Cases = append(suite.Cases, *testCase)
	} else {
		err := reset(jirix, mirror, projects)
		testCase := makeTestCase("reset", err)
		if err != nil {
			suite.Failures++
		}
		suite.Cases = append(suite.Cases, *testCase)
	}

	err := push(jirix, mirror, projects)
	testCase := makeTestCase("push", err)
	if err != nil {
		suite.Failures++
	}
	suite.Cases = append(suite.Cases, *testCase)

	return &suite, nil
}

func makeTestCase(action string, err error) *xunit.TestCase {
	c := xunit.TestCase{
		Classname: "git",
		Name:      action,
	}

	if err != nil {
		f := xunit.Failure{
			Message: "git error",
			Data:    fmt.Sprintf("%v", err),
		}
		c.Failures = append(c.Failures, f)
	}

	return &c
}

func clone(jirix *jiri.X, mirror Mirror, projects string) error {
	dirname := filepath.Join(projects, mirror.name)
	return gitutil.New(jirix.NewSeq()).CloneRecursive(mirror.googlesource, dirname)
}

func reset(jirix *jiri.X, mirror Mirror, projects string) error {
	dirname := filepath.Join(projects, mirror.name)
	rootOpt := gitutil.RootDirOpt(dirname)
	git := gitutil.New(jirix.NewSeq(), rootOpt)

	// Fetch master branch from origin.
	if err := git.FetchRefspec("origin", "master", gitutil.TagsOpt(true)); err != nil {
		return err
	}

	// Reset local master to origin/master.
	return git.Reset("origin/master")
}

func push(jirix *jiri.X, mirror Mirror, projects string) error {
	dirname := filepath.Join(projects, mirror.name)
	opts := gitutil.RootDirOpt(dirname)
	return gitutil.New(jirix.NewSeq(), opts).Push(mirror.github, "master", gitutil.ForceOpt(true), gitutil.FollowTagsOpt(mirror.followTags))
}
