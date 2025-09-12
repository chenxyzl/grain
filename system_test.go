package grain

import (
	"strconv"
	"testing"
)

func BenchmarkCalcPos(b *testing.B) {
	x := &system{addrHash: newAddrHash()}
	var clusterNodes []tNodeState
	for i := 0; i < 20; i++ {
		clusterNodes = append(clusterNodes, tNodeState{
			NodeId:  uint64(i + 1),
			Address: "aaaaa" + strconv.Itoa(i+1),
			Version: "aaaaa" + strconv.Itoa(i+1),
			Time:    "aaaaa" + strconv.Itoa(i+1),
			Kinds:   []string{"player1", "player2", "player3", "player4", "player5"},
		})
	}
	b.ResetTimer()
	var v string
	for n := 0; n < b.N; n++ {
		tmp := x.getAddrHash().CalcAddressByKind8Id(clusterNodes, "player3", "testname")
		if v == "" {
			v = tmp
		}
		if v != tmp {
			b.Error("CalcAddressByKind8Id failed", v)
		}
	}
}
