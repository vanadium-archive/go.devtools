// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testinternal

import (
	"v.io/x/devtools/godepcop/testdata/test-internal/internal"
	"v.io/x/devtools/godepcop/testdata/test-internal/internal/child"
)

func main() {
	internal.A()
	child.C()
}
