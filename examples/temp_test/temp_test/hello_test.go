package temp_test

import (
	"fmt"
	"testing"
)

type testCase struct {
	data map[int]int
}

var testCases = &testCase{data: map[int]int{1: 1, 2: 2, 3: 3, 4: 4}}

func getMap() map[int]int {
	return testCases.data
}

func TestHello(t *testing.T) {
	a := getMap()
	for i := 1; i <= 2; i++ {
		delete(a, i)
	}
	fmt.Println(testCases.data)
}
