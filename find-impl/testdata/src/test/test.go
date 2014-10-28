package test

type Interface interface {
	Test()
}

type Implementation struct{}

func (Implementation) Test() {}
