package func_test

import (
	"errors"
	"fmt"
	"testing"
)

func f() error {
	var x error = errors.New("xxx")
	defer func() {
		if err := recover(); err != nil {
			fmt.Println(err)
		}
		x = errors.New("aaaaa")
	}()
	defer func() {
		fmt.Println(1)
		x = errors.New("bbbbb")
	}()
	//panic("err")
	return x
}

func TestPanic(t *testing.T) {
	fmt.Println(f())
}
