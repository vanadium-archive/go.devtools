// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package passeschecks

import "v.io/x/ref/lib/apilog"

type SplitType struct{}

func (SplitType) Method1() {
	// does not make a difference to have a
	// random comment
	// here
	defer apilog.LogCall("some more random text")()
	// or here
}
