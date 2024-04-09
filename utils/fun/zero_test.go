package fun

import (
	"fmt"
	"testing"
)

func TestZero(t *testing.T) {
	type ta struct{ x int }
	v1 := Zero[ta]()
	v1.x = 1
	fmt.Print(v1)
	v2 := Zero[*ta]()
	v2.x = 2
	fmt.Print(v2)
	v3 := Zero[int]()
	fmt.Print(v3)
	v4 := Zero[*int]()
	fmt.Print(v4)
}
