// test7 tests whether types with functions receiving
// pointers are implementing the logging constructs
// correctly.
package test7

type PtrType struct{}

func (*PtrType) Method1()    {}
func (*PtrType) Method2(int) {}
