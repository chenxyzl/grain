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

type testStruct struct{ a int }

func TestMapKey(t *testing.T) {
	var m map[*testStruct]int
	m = make(map[*testStruct]int)
	m[&testStruct{1}] = 1
	m[&testStruct{2}] = 2
	m[&testStruct{1}] = 1
	v := &testStruct{4}
	m[v] = 5
	m[v] = 6
	fmt.Println(m)
}
