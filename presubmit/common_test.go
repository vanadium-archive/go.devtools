// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "testing"

func TestGenStartPresubmitBuildLink(t *testing.T) {
	strRefs := "refs/changes/12/1012/1:refs/changes/13/1013/1"
	strProjects := "release.go.core:release.go.tools"
	strTests := "vanadium-go-test vanadium-go-race"

	got := genStartPresubmitBuildLink(strRefs, strProjects, strTests)
	want := "https://veyron.corp.google.com/jenkins/job/vanadium-presubmit-test/buildWithParameters?REFS=refs%2Fchanges%2F12%2F1012%2F1%3Arefs%2Fchanges%2F13%2F1013%2F1&PROJECTS=release.go.core%3Arelease.go.tools&TESTS=vanadium-go-test+vanadium-go-race"
	if want != got {
		t.Fatalf("\nwant:\n%s\n\ngot:\n%s\n", want, got)
	}
}
