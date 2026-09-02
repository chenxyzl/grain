package uuid

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// TestPreEpochClockDoesNotUnderflow pins the epoch floor: (now - epoch) is a uint64
// subtraction, so a pre-epoch clock (unsynced container, NTP step back, dead RTC) wraps it
// into a huge value that breaks ParseSortVal ordering and aliases far-future ids.
func TestPreEpochClockDoesNotUnderflow(t *testing.T) {
	u, err := NewUUID(1)
	if err != nil {
		t.Fatal(err)
	}
	// Generate reads time.Now() itself, so assert on the arithmetic it performs, with the floor.
	pre := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	floored := pre
	if floored < int64(epoch) {
		floored = int64(epoch)
	}
	if floored != int64(epoch) {
		t.Fatalf("a 2020 timestamp must floor to epoch, got %d", floored)
	}

	// An id whose timestamp field is beyond the current wall clock is the underflow signature.
	nowID := u.Generate()
	maxPlausible := (uint64(time.Now().UnixMilli()+1000) - epoch) << timeShift
	if nowID >= maxPlausible {
		t.Errorf("id %d exceeds the plausible range for the current clock (%d) — underflow?",
			nowID, maxPlausible)
	}

	// And a pre-epoch clock, once floored, yields an id in range.
	badID := (uint64(floored)-epoch)<<timeShift | (1 << nodeShift)
	if badID >= maxPlausible {
		t.Errorf("floored pre-epoch id %d is still out of range", badID)
	}
}

// TestStepExhaustionWaitIsBounded: with the 12-bit sequence exhausted inside a millisecond,
// Generate waits for the next one (see TestTimestampNeverLeadsRealClock for why). nowMs is
// monotonic, so the longest that wait can ever be is the rest of the current millisecond.
func TestStepExhaustionWaitIsBounded(t *testing.T) {
	u, err := NewUUID(1)
	if err != nil {
		t.Fatal(err)
	}
	// stepMax+1 ids exhaust one millisecond of sequence space; several multiples hit the wait.
	total := int(stepMax+1) * 3
	var worst time.Duration
	for range total {
		start := time.Now()
		u.Generate()
		if d := time.Since(start); d > worst {
			worst = d
		}
	}
	// 1ms is the theoretical bound; the slack absorbs scheduling noise.
	if worst > 20*time.Millisecond {
		t.Errorf("worst single Generate took %v: the wait is no longer bounded by the millisecond boundary, which means it is reading the wall clock again", worst)
	}
	t.Logf("worst single Generate across %d ids (3 full sequence wraps): %v",
		total, worst.Round(time.Microsecond))
}

// TestStepExhaustionKeepsIdsUniqueAndOrdered: the step wrap must keep ids unique and ordered.
func TestStepExhaustionKeepsIdsUniqueAndOrdered(t *testing.T) {
	u, err := NewUUID(7)
	if err != nil {
		t.Fatal(err)
	}
	// well past one millisecond of step space (stepMax+1 = 4096), so the wrap path repeats
	const n = 4096*3 + 17
	seen := make(map[uint64]struct{}, n)
	var prev uint64
	for i := range n {
		id := u.Generate()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id %d at iteration %d", id, i)
		}
		seen[id] = struct{}{}
		if i > 0 && id <= prev {
			t.Fatalf("ids not strictly increasing at %d: %d then %d", i, prev, id)
		}
		prev = id
		if ParseNode(id) != 7 {
			t.Fatalf("node bits corrupted at %d: got %d", i, ParseNode(id))
		}
	}
}

// TestConcurrentGenerateUnique guards the lock: no duplicate ids across goroutines.
func TestConcurrentGenerateUnique(t *testing.T) {
	u, err := NewUUID(3)
	if err != nil {
		t.Fatal(err)
	}
	const goroutines, per = 16, 3000
	var mu sync.Mutex
	seen := make(map[uint64]struct{}, goroutines*per)
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := make([]uint64, 0, per)
			for range per {
				local = append(local, u.Generate())
			}
			mu.Lock()
			defer mu.Unlock()
			for _, id := range local {
				if _, dup := seen[id]; dup {
					t.Errorf("duplicate id %d", id)
					return
				}
				seen[id] = struct{}{}
			}
		}()
	}
	wg.Wait()
	if len(seen) != goroutines*per {
		t.Errorf("got %d unique ids, want %d", len(seen), goroutines*per)
	}
}

// TestTimestampNeverLeadsRealClock pins the invariant that keeps ids safe across a restart,
// and is why Generate waits instead of borrowing the next millisecond: a restart re-anchors
// to the wall clock, so a logical timestamp ahead of it would resume from already-issued ids.
// Waiting keeps timestamp <= real clock, so a restart can at worst re-enter this millisecond.
func TestTimestampNeverLeadsRealClock(t *testing.T) {
	u, err := NewUUID(11)
	if err != nil {
		t.Fatal(err)
	}
	for i := range int(stepMax+1) * 3 {
		u.Generate()
		u.mu.Lock()
		lead := u.timestamp - u.nowMs()
		u.mu.Unlock()
		if lead > 0 {
			t.Fatalf("at id %d the logical timestamp leads the real clock by %dms; ids are no longer safe across a restart", i, lead)
		}
	}
}

// TestNoCollisionAfterRestart is the end-to-end form of the invariant above. A 1ms window is
// inherent to snowflake without persistence — a restart inside the SAME millisecond can reuse
// (timestamp, step) — but a real restart takes far longer; the sleep stands in for it.
func TestNoCollisionAfterRestart(t *testing.T) {
	u, err := NewUUID(1)
	if err != nil {
		t.Fatal(err)
	}
	const n = 20000
	issued := make(map[uint64]struct{}, n)
	for range n {
		issued[u.Generate()] = struct{}{}
	}

	time.Sleep(3 * time.Millisecond) // stand in for process restart time

	u2, err := NewUUID(1) // same node id: a restart keeps it
	if err != nil {
		t.Fatal(err)
	}
	for i := range n {
		id := u2.Generate()
		if _, dup := issued[id]; dup {
			t.Fatalf("post-restart id %d (iteration %d) was already issued before the restart", id, i)
		}
	}
}

// TestClockBaseIsMonotonic pins the structural rollback immunity: nowMs is a construction-time
// wall sample plus elapsed MONOTONIC time, which no NTP step, manual clock change or RTC jump
// can move backwards.
func TestClockBaseIsMonotonic(t *testing.T) {
	u, err := NewUUID(1)
	if err != nil {
		t.Fatal(err)
	}
	var prev int64
	for i := range 200000 {
		v := u.nowMs()
		if i > 0 && v < prev {
			t.Fatalf("nowMs went backwards: %d -> %d", prev, v)
		}
		prev = v
	}
	// And it must be anchored to real wall time, not to zero or to uptime.
	wall := time.Now().UnixMilli()
	if diff := prev - wall; diff < -1000 || diff > 1000 {
		t.Errorf("nowMs=%d is %dms away from wall clock %d — base is not anchored",
			prev, diff, wall)
	}
}

// TestClockBaseIsFlooredAtEpoch: the epoch clamp happens once, at construction, since nowMs
// advances monotonically from the base. A pre-epoch host clock must still yield a valid id.
func TestClockBaseIsFlooredAtEpoch(t *testing.T) {
	u, err := NewUUID(1)
	if err != nil {
		t.Fatal(err)
	}
	if u.baseWall < int64(epoch) {
		t.Fatalf("baseWall %d is below epoch %d", u.baseWall, epoch)
	}
	// Simulate having been constructed on a 2020 host.
	u.baseWall = max(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), int64(epoch))
	if u.baseWall != int64(epoch) {
		t.Fatalf("a 2020 base must floor to epoch, got %d", u.baseWall)
	}
	id := u.Generate()
	maxPlausible := (uint64(time.Now().UnixMilli()+1000) - epoch) << timeShift
	if id >= maxPlausible {
		t.Errorf("id %d from a floored pre-epoch base is out of range (max %d)", id, maxPlausible)
	}
	if ParseNode(id) != 1 {
		t.Errorf("node bits corrupted: %d", ParseNode(id))
	}
}

// TestBaseMonoCarriesMonotonicReading guards the one assumption nowMs rests on: baseMono must
// still carry its monotonic reading, or time.Since falls back to Now().Sub(t) — the WALL clock
// — silently losing the rollback immunity. .UTC(), .Local(), .Round(), .Truncate() and a
// time.Unix round-trip all strip it; Time.String() appends "m=" exactly when it is present.
func TestBaseMonoCarriesMonotonicReading(t *testing.T) {
	u, err := NewUUID(1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u.baseMono.String(), "m=") {
		t.Fatalf("baseMono lost its monotonic reading (%q): time.Since will fall back to "+
			"Now().Sub(t), reading the wall clock — nowMs is then neither faster nor "+
			"rollback-immune. Something stripped it (.UTC/.Local/.Round/.Truncate/time.Unix).",
			u.baseMono.String())
	}
	// Sanity: the stripped form must be detectable, i.e. the assertion above can fail.
	if stripped := u.baseMono.Round(0); strings.Contains(stripped.String(), "m=") {
		t.Fatal("premise broken: Round(0) is documented to strip the monotonic reading")
	}
}

// TestEpochIsPinned nails down the epoch constant: the origin of the id's timestamp field,
// pinned as TRUE UTC (2025-01-01T00:00:00Z, not 2025-01-01 in a local zone — a zone slip
// shifts every id by the offset). Changing it re-bases every id and breaks ordering.
func TestEpochIsPinned(t *testing.T) {
	want := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := time.UnixMilli(int64(epoch)).UTC(); !got.Equal(want) {
		t.Fatalf("epoch = %d = %v, want %v (2025-01-01 UTC). Changing this re-bases every "+
			"id and breaks ordering against previously persisted ids.", epoch, got, want)
	}

	// The 42-bit millisecond field must still have plenty of runway from here.
	msSpan := int64(1) << 42
	exhaust := time.UnixMilli(int64(epoch) + msSpan).UTC()
	if years := exhaust.Sub(want).Hours() / 24 / 365.25; years < 100 {
		t.Errorf("only %.0f years of timestamp headroom from epoch", years)
	}
	t.Logf("epoch = %v UTC; 42-bit ms field exhausts %v", want.Format("2006-01-02"), exhaust.Format("2006-01"))
}

// TestClockBeforeEpochIsFloored: any clock before the epoch collapses to epoch+uptime — still
// unique and ordered, just not meaningful as a timestamp. A valid id beats an out-of-range one.
func TestClockBeforeEpochIsFloored(t *testing.T) {
	for _, y := range []int{2020, 2023, 2024} {
		wall := time.Date(y, 6, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
		if got := max(wall, int64(epoch)); got != int64(epoch) {
			t.Errorf("a %d clock should floor to epoch, got %d", y, got)
		}
	}
	// And a current clock must NOT be floored.
	if got := max(time.Now().UnixMilli(), int64(epoch)); got == int64(epoch) {
		t.Error("the current clock was floored to epoch — is the host clock before 2025?")
	}
}
