package passeschecks

import "v.io/x/lib/vlog"

type SplitType struct{}

func (SplitType) Method1() {
	// does not make a difference to have a
	// random comment
	// here
	defer vlog.LogCall("random text")()
	// or here
}
