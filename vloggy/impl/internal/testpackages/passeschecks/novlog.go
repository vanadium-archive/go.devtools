// +build testpackage

package passeschecks

import "fmt"

type NoVLogType1 struct{}

func (NoVLogType1) Method1() {
	// comment can go here
	//novlog
	fmt.Println()
	// other comment
}

func (NoVLogType1) NotPartOfInterfaceA() {}

func (NoVLogType1) Method2(int) {
	// novlog
}

func (NoVLogType1) NotPartOfInterfaceB() {
	fmt.Println("does not need log")
}

type NoVLogHalfType2 struct{}

func (NoVLogHalfType2) Method1() {
	// random comment here
	//    novlog
}

func (NoVLogHalfType2) NotPartOfInterfaceMethod3() {
}

type NoVLogHalfType3 struct {
	NoVLogHalfType2
}

func (NoVLogHalfType3) Method2(int) {
	//novlog
}

type NoVLogType3 struct{}

func (NoVLogType3) Method1() {}
