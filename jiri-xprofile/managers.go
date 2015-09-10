// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	// Add profile manager implementations here.
	_ "v.io/x/devtools/jiri-xprofile/android"
	_ "v.io/x/devtools/jiri-xprofile/base"
	_ "v.io/x/devtools/jiri-xprofile/go"
	_ "v.io/x/devtools/jiri-xprofile/java"
	_ "v.io/x/devtools/jiri-xprofile/nacl"
	_ "v.io/x/devtools/jiri-xprofile/nodejs"
	_ "v.io/x/devtools/jiri-xprofile/syncbase"
)
