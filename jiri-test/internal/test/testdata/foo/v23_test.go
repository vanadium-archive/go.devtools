// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package foo_test

import (
	"testing"

	"v.io/x/ref/test/v23test"
)

func TestV23(t *testing.T) {
	v23test.SkipUnlessRunningIntegrationTests(t)
}

func TestV23B(t *testing.T) {
	v23test.SkipUnlessRunningIntegrationTests(t)
}

func TestV23Hello(t *testing.T) {
	v23test.SkipUnlessRunningIntegrationTests(t)
}

func TestMain(m *testing.M) {
	v23test.TestMain(m)
}
