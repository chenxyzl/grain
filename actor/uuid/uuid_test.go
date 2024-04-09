package uuid

import (
	"fmt"
	"testing"
)

func TestGenerate(t *testing.T) {
	Init(1000)
	v := Generate()
	v1 := ParseSortVal(v)
	v2 := ParseNode(v)
	v3 := ParseStep(v)
	v4 := ParseTime(v)
	if v2 != 1000 {
		t.Error()
	}
	if v3 != 0 {
		t.Error()
	}
	v5 := v4<<timeShift | (v2 << nodeShift) | (v3)
	if v5 != v {
		t.Error()
	}
	fmt.Println(v1)
}
