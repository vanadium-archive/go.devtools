// +build testpackage

package passeschecks

import "fmt"

type NoLogCallType1 struct{}

func (NoLogCallType1) Method1() {
	// comment can go here
	//nologcall
	fmt.Println()
	// other comment
}

func (NoLogCallType1) NotPartOfInterfaceA() {}

func (NoLogCallType1) Method2(int) {
	// nologcall
}

func (NoLogCallType1) NotPartOfInterfaceB() {
	fmt.Println("does not need log")
}

type NoLogCallHalfType2 struct{}

func (NoLogCallHalfType2) Method1() {
	// random comment here
	//    nologcall
}

func (NoLogCallHalfType2) NotPartOfInterfaceMethod3() {
}

type NoLogCallHalfType3 struct {
	NoLogCallHalfType2
}

func (NoLogCallHalfType3) Method2(int) {
	/* nologcall */
}

type NoLogCallType3 struct{}

func (NoLogCallType3) Method1() {}
