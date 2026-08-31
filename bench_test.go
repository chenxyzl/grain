package grain

import (
	"sync/atomic"
	"testing"

	"github.com/chenxyzl/grain/message"
)

// These microbenchmarks drive the REAL routing path — system.tellWithSender ->
// sendToLocal -> registry.get -> proc.send -> ringbuffer.Push — with no etcd and no
// grpc, so they can actually resolve a 20ns change.
//
// examples/benchmark_test/actor_test is the end-to-end benchmark, but it spawns 10k
// actors, talks to etcd and runs 32-way parallel; its within-version variance exceeds
// 2x, which is wider than every optimisation being attempted here.

// sink drains messages without doing any work, so timings reflect the send path.
type sink struct{ BaseActor }

func (a *sink) Started()            {}
func (a *sink) PreStop()            {}
func (a *sink) Receive(ctx Context) {}

func benchSystem(tb testing.TB) (*system, ActorRef, *int64) {
	sys := newTestSystemTB(tb)
	// The producer outruns the drainer (per-message drain cost is comparable to send
	// cost), so the mailbox fills. Swallow overflows in a counter instead of the default
	// WARN log, otherwise the benchmark measures slog rather than the send path.
	overflow := new(int64)
	sys.config.deadLetterHandler = func(DeadLetter) { atomic.AddInt64(overflow, 1) }
	// A production-shaped name: ids are "direct/local/<uuid>@<host:port>" (~50 chars),
	// and the shard hash cost scales with that length.
	ref, err := sys.SpawnNamed(func() IActor { return &sink{} }, "484024768387878912",
		WithOptsMailboxSize(1024), WithOptsMailboxMaxSize(1<<20))
	if err != nil {
		tb.Fatal(err)
	}
	return sys, ref, overflow
}

// BenchmarkTellLocal is the send hot path: one Tell to a local, live actor.
func BenchmarkTellLocal(b *testing.B) {
	sys, ref, overflow := benchSystem(b)
	msg := &message.Unsubscribe{EventName: "x"}
	var n int64
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		sys.tellWithSender(ref, msg, nil, 1)
		n++
	}
	b.StopTimer()
	// The producer outruns the drainer, so past a point every send dead-letters instead
	// of enqueueing. Report the share: a high ratio means this measures the overflow
	// path (which still goes through routing + registry.get + Push) rather than a clean
	// enqueue, and the absolute ns/op should be read with that in mind.
	b.ReportMetric(float64(atomic.LoadInt64(overflow))/float64(n)*100, "%overflow")
}

// BenchmarkRegistryGet isolates the registry lookup, which the audit measured at ~32%
// of the send path (id string -> byte-at-a-time fnv32 -> map hash of the same string).
func BenchmarkRegistryGet(b *testing.B) {
	sys, ref, _ := benchSystem(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if sys.registry.get(ref) == nil {
			b.Fatal("missing")
		}
	}
}

// BenchmarkRingBufferPushPop isolates the mailbox, where the index advance uses a
// hardware DIV (`% rb.cap`) twice per message, inside the mutex.
func BenchmarkRingBufferPushPop(b *testing.B) {
	sys, ref, _ := benchSystem(b)
	proc := sys.registry.get(ref).(*processorMailBox)
	ctx := newContext(ref, nil, &message.Unsubscribe{EventName: "x"}, 1, sys)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		proc.rb.Push(ctx)
		proc.rb.Pop()
	}
}

// BenchmarkCalcAddr covers cluster routing: called per cache miss on the send path and
// per actor on every membership change.
func BenchmarkCalcAddr(b *testing.B) {
	h := newAddrHash()
	nodes := make([]tNodeState, 0, 20)
	for i := range 20 {
		nodes = append(nodes, tNodeState{
			NodeId:  uint64(i + 1),
			Address: "10.10.108." + string(rune('a'+i)) + ":5000" + string(rune('0'+i%10)),
			Kinds:   []string{"player"},
		})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if h.CalcAddrByKind8Name(nodes, "player", "484024768387878912") == "" {
			b.Fatal("no owner")
		}
	}
}

// replier answers every request, so Ask completes a full round trip.
type benchReplier struct{ BaseActor }

func (a *benchReplier) Started() {}
func (a *benchReplier) PreStop() {}
func (a *benchReplier) Receive(ctx Context) {
	if _, ok := ctx.Message().(*message.Subscribe); ok {
		ctx.Reply(&message.Unsubscribe{EventName: "r"})
	}
}

// BenchmarkAskLocal is the request/reply hot path: correlation id, reply channel,
// replyRef, send, drain, reply, await. The audit attributes 6 allocs/op to it —
// the reply chan (2), the context, the replyRef, and the per-Ask drainer goroutine
// plus its drainState.
func BenchmarkAskLocal(b *testing.B) {
	sys := newTestSystemTB(b)
	ref, err := sys.SpawnNamed(func() IActor { return &benchReplier{} }, "484024768387878913")
	if err != nil {
		b.Fatal(err)
	}
	req := &message.Subscribe{EventName: "q"}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, e := NoReentryAsk[*message.Unsubscribe](ref, req); e != nil {
			b.Fatal(e)
		}
	}
}

// BenchmarkSpawn puts uuid.Generate's cost in context: Spawn is its only caller on any
// hot-ish path (one id per actor).
//
// Measured ~4.2us / 3.9KB / 23 allocs per spawn, against uuid.Generate at 47.8ns — i.e.
// the id generator is ~1% of a spawn, and the mutex inside it ~0.2%. Replacing that lock
// with a CAS loop measured 41.5ns (-13%), which is ~0.15% of a spawn: not worth
// rewriting the clock-rollback and uniqueness logic in a lock-free setting. Recorded here
// so the question does not have to be re-litigated from intuition.
func BenchmarkSpawn(b *testing.B) {
	sys := newTestSystemTB(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		sys.Spawn(func() IActor { return &sink{} })
	}
}
