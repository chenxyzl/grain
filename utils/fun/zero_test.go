package fun

import (
	"testing"
)

func TestZero(t *testing.T) {
	type ta struct{ x int }
	v1 := Zero[ta]()
	v1.x = 1

	v2 := Zero[*ta]()
	if v2 != nil {
		t.Error()
	}

	v3 := Zero[int]()
	v3 = 3
	_ = v3

	v4 := Zero[*int]()
	if v4 != nil {
		t.Error()
	}
}
