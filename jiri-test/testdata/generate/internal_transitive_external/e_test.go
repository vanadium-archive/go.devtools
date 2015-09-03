// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal_transitive_external

import (
	"testing"

	_ "v.io/x/ref/runtime/factories/generic"
)

func TestModulesExternal(t *testing.T) {
	Module(t)
}
