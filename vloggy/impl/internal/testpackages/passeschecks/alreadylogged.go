// passeschecks should pass all the log checks, as it includes
// all the necessary logging constructs.
package passeschecks

import "veyron.io/veyron/veyron2/vlog"

type Type1 struct{}

func (Type1) Method1() {
	// random comment
	defer vlog.LogCall("random text")()
}

func (Type1) Method2(int) {
	defer vlog.LogCall()()
}

func (Type1) NotPartOfInterfaceMethod3() {}

type NotImplementingInterface struct{}

func (NotImplementingInterface) Method1() {}
func (NotImplementingInterface) Method2() {}
