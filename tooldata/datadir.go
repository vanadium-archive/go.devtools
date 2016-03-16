// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tooldata

import (
	"path/filepath"

	"v.io/jiri"
)

// DataDirPath returns the path to the data directory of the given tool.
func DataDirPath(jirix *jiri.X, toolName string) (string, error) {
	return filepath.Join(jirix.Root, "release", "go", "src", "v.io", "x", "devtools", "tooldata", "data"), nil
}
