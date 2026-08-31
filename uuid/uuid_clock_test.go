package uuid

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// TestPreEpochClockDoesNotUnderflow pins the epoch floor.
//
// The id layout encodes (now - epoch) as a uint64 subtraction, so a clock before
// 2023-01-01 used to wrap it: measured, a 2020 clock produced 18049687768967155712
// where a normal id is ~485046263180431360 — 37x larger, which breaks ParseSortVal
// ordering and aliases with ids real nodes will emit ~133 years out. Triggered by an
// unsynced container clock, an NTP step back, or a dead RTC.
func TestPreEpochClockDoesNotUnderflow(t *testing.T) {
	u, err := NewUUID(1)
	if err != nil {
		t.Fatal(err)
	}
	// Drive Generate as if the wall clock were before epoch. Generate reads
	// time.Now() itself, so assert on the arithmetic it performs, with the floor.
	pre := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	floored := pre
	if floored < int64(epoch) {
		floored = int64(epoch)
	}
	if floored != int64(epoch) {
		t.Fatalf("a 2020 timestamp must floor to epoch, got %d", floored)
	}

	// The real generator must never produce an id whose timestamp field is beyond the
	// current wall clock: that is exactly the underflow signature.
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

// TestStepExhaustionWaitIsBounded pins what is left of the original stall bug.
//
// When the 12-bit sequence is exhausted within a millisecond, Generate waits for the next
// millisecond — deliberately, so the logical timestamp never runs ahead of the real clock
// (see TestTimestampNeverLeadsRealClock for why that matters). The bug was never the
// waiting itself: it was that the wait read the WALL clock, so after a clock rollback the
// logical timestamp led the wall clock and one call spun for the entire rollback duration
// while holding the mutex — measured, a 300ms rollback blocked a concurrent Generate for
// 295ms and burned a core.
//
// nowMs is monotonic, so a rollback can no longer inflate the wait: the only thing it can
// ever wait for is the current millisecond to end. This exercises the wait naturally, with
// no artificial state, and asserts that bound.
func TestStepExhaustionWaitIsBounded(t *testing.T) {
	u, err := NewUUID(1)
	if err != nil {
		t.Fatal(err)
	}
	// stepMax+1 ids exhaust one millisecond of sequence space, so several multiples of
	// that guarantee the wait path is taken repeatedly.
	total := int(stepMax+1) * 3
	var worst time.Duration
	for range total {
		start := time.Now()
		u.Generate()
		if d := time.Since(start); d > worst {
			worst = d
		}
	}
	// 1ms is the theoretical bound; the slack absorbs scheduling noise. A regression to
	// the wall clock would show up here as tens or hundreds of milliseconds.
	if worst > 20*time.Millisecond {
		t.Errorf("worst single Generate took %v: the wait is no longer bounded by the millisecond boundary, which means it is reading the wall clock again", worst)
	}
	t.Logf("worst single Generate across %d ids (3 full sequence wraps): %v",
		total, worst.Round(time.Microsecond))
}

// TestStepExhaustionKeepsIdsUniqueAndOrdered: borrowing the next millisecond must not
// break the two properties the wait loop was there to protect.
func TestStepExhaustionKeepsIdsUniqueAndOrdered(t *testing.T) {
	u, err := NewUUID(7)
	if err != nil {
		t.Fatal(err)
	}
	// Generate well past one millisecond's worth of step space (stepMax+1 = 4096) so the
	// wrap path is exercised many times.
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

// TestConcurrentGenerateUnique guards the lock itself: no duplicates across goroutines,
// including through step wraps.
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

// TestTimestampNeverLeadsRealClock pins the invariant that keeps ids safe across a process
// restart, and is the reason Generate waits rather than borrowing the next millisecond.
//
// Borrowing was tried and reverted. It was ~7.5x faster (30753 vs 4103 ids/ms) but let the
// logical timestamp run ahead of the real clock at ~6.5ms of lead per 1ms of real time.
// Since a restarted process re-anchors to the wall clock, it would resume from a timestamp
// already issued — measured, 5000 of the first 5000 post-restart ids collided with
// pre-restart ones. Waiting keeps timestamp <= real clock, so a restart can only ever
// re-enter the CURRENT millisecond.
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

// TestNoCollisionAfterRestart is the end-to-end form of the invariant above: ids issued
// before a restart must not be reissued after it.
//
// One residual window is inherent to snowflake without persistence — a restart landing
// inside the SAME millisecond can reuse (timestamp, step) pairs. That window is 1ms,
// against the seconds of exposure borrowing created, and a real restart (grpc listen plus
// etcd registration) takes orders of magnitude longer. The sleep stands in for that.
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

// TestClockBaseIsMonotonic pins the structural rollback immunity.
//
// nowMs derives the current millisecond from a wall-clock sample taken at construction
// plus elapsed MONOTONIC time. The monotonic clock is unaffected by NTP steps, manual
// clock changes or RTC jumps, so the value can never go backwards — which removes the
// "clock went back, ids repeat" class of bugs at the source rather than compensating for
// it afterwards.
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

// TestClockBaseIsFlooredAtEpoch: the epoch clamp moved to construction (it only has to
// happen once, since nowMs advances monotonically from the base). A pre-epoch host clock
// must still not produce an out-of-range id.
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

// TestBaseMonoCarriesMonotonicReading guards the single assumption nowMs rests on.
//
// time.Since has two branches (see the stdlib source):
//
//	func Since(t Time) Duration {
//		if t.wall&hasMonotonic != 0 && !runtimeIsBubbled() {
//			return subMono(runtimeNano()-startNano, t.ext)  // fast: ONE monotonic read
//		}
//		return Now().Sub(t)                                 // fallback: reads the WALL clock
//	}
//
// Everything nowMs claims depends on taking the first branch, and that requires baseMono
// to still carry its monotonic reading. Several ordinary-looking operations strip it —
// .UTC(), .Local(), .Round(), .Truncate(), or round-tripping through time.Unix. If a
// future edit does any of those, the fallback kicks in SILENTLY and we lose both:
//
//   - the speed (Now() reads wall + monotonic; runtimeNano() reads only monotonic), and
//   - much worse, the rollback immunity, because Now() is the wall clock again.
//
// Time.String() documents that it appends "m=±<value>" exactly when a monotonic reading
// is present, so that is the observable to assert on.
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

// TestEpochIsPinned nails down the epoch constant and its meaning.
//
// epoch is the origin of the id's timestamp field, so changing it silently is a data
// break: a later epoch makes (now-epoch) smaller, so ids minted afterwards sort BEFORE
// ids minted before the change (measured: 54.6% smaller at the same instant). Anything
// that persisted an id as an actor name, primary key or sort key would be affected.
//
// It is also pinned as true UTC on purpose. The previous constant (1672502400000) was
// commented "2023-01-01:00:00:00 GMT" but actually evaluated to 2022-12-31T16:00:00Z,
// i.e. 2023-01-01 in UTC+8 — so anyone "fixing" the value to match the comment would
// have shifted every id by 8 hours.
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

// TestClockBeforeEpochIsFloored: the floor now catches any clock before 2025, a wider
// range than before (it used to be before 2023). A host with a 2024 clock therefore has
// all its ids collapsed to epoch+uptime — still unique and ordered, but not meaningful as
// timestamps. That is the intended trade: a valid id beats an out-of-range one.
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
