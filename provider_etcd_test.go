package grain

import (
	"strconv"
	"testing"

	"github.com/chenxyzl/grain/uuid"
)

// newMemberProvider builds a provider whose nodeMap shows the given ids as taken, keyed the
// way parseWatch keys it (the last path segment, i.e. the id itself).
func newMemberProvider(takenIds ...uint64) *providerEtcd {
	p := &providerEtcd{nodeMap: map[string]tNodeState{}}
	for _, id := range takenIds {
		p.nodeMap[strconv.FormatUint(id, 10)] = tNodeState{NodeId: id}
	}
	return p
}

func TestFreeNodeIds(t *testing.T) {
	max := uuid.MaxNodeMax()

	all := newMemberProvider().freeNodeIds()
	if uint64(len(all)) != max {
		t.Errorf("with nothing taken, %d ids are free, want %d", len(all), max)
	}
	if all[0] != 1 || all[len(all)-1] != max {
		t.Errorf("claimable range is [%d, %d], want [1, %d]", all[0], all[len(all)-1], max)
	}

	free := newMemberProvider(1, 2, 7).freeNodeIds()
	if uint64(len(free)) != max-3 {
		t.Errorf("3 taken leaves %d free, want %d", len(free), max-3)
	}
	for _, id := range free {
		if id == 1 || id == 2 || id == 7 {
			t.Fatalf("taken id %d is still offered as free", id)
		}
	}

	fullIds := make([]uint64, 0, max)
	for id := uint64(1); id <= max; id++ {
		fullIds = append(fullIds, id)
	}
	if got := newMemberProvider(fullIds...).freeNodeIds(); len(got) != 0 {
		t.Errorf("a full cluster must offer no free ids, got %d", len(got))
	}
}

// The claimable range has to stay inside uuid's node field, or a node publishes an id to
// etcd and then panics on uuid.Init with it.
func TestClaimableNodeIdsAreAcceptedByUuid(t *testing.T) {
	// uuid.Init mutates process-global generator state other tests rely on
	t.Cleanup(func() { _ = uuid.Init(1) })

	free := newMemberProvider().freeNodeIds()
	for _, id := range []uint64{free[0], free[len(free)-1]} {
		if err := uuid.Init(id); err != nil {
			t.Errorf("claimable node id %d rejected by uuid.Init: %v", id, err)
		}
	}
	if err := uuid.Init(uuid.MaxNodeMax() + 1); err == nil {
		t.Error("uuid.Init should reject an id above MaxNodeMax; the claimable range is only " +
			"safe because it stops there")
	}
}
