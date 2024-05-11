package func_test

import (
	"fmt"
	"testing"
)

type testCase struct {
	a int
}

func f() (v *testCase) {
	v = &testCase{}
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("panic")
			v.a = 111
		}
	}()
	panic("err1111")
	fmt.Println("1")
	return v
}

func TestPanic(t *testing.T) {
	a := f()
	fmt.Println(a.a)
	fmt.Println(2)
	fmt.Println(3)
}
