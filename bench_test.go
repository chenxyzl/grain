package grain

import (
	"sync/atomic"
	"testing"

	"github.com/chenxyzl/grain/message"
)

// These drive the real routing path (tellWithSender -> sendToLocal -> registry.get ->
// proc.send -> Push) with no etcd and no grpc, so they resolve small changes;
// examples/benchmark_test is the end-to-end counterpart, with >2x within-version variance.

// sink drains messages without doing any work, so timings reflect the send path.
type sink struct{ BaseActor }

func (a *sink) Started()            {}
func (a *sink) PreStop()            {}
func (a *sink) Receive(ctx Context) {}

func benchSystem(tb testing.TB) (*system, ActorRef, *int64) {
	sys := newTestSystemTB(tb)
	// producer outruns drainer, so count overflows instead of benchmarking the default WARN log
	overflow := new(int64)
	sys.config.deadLetterHandler = func(DeadLetter) { atomic.AddInt64(overflow, 1) }
	// production-shaped name: ids are ~50 chars, and shard hash cost scales with length
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
	// high %overflow means ns/op describes the dead-letter path, not a clean enqueue
	b.ReportMetric(float64(atomic.LoadInt64(overflow))/float64(n)*100, "%overflow")
}

// BenchmarkRegistryGet isolates the registry lookup, ~32% of the send path.
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

// BenchmarkRingBufferPushPop isolates the mailbox enqueue/dequeue under its mutex.
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

// BenchmarkCalcAddr covers cluster routing: per send-path cache miss, and per actor on every
// membership change.
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

// BenchmarkAskLocal is the full round trip: correlation id, reply chan, replyRef, send, await.
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

// BenchmarkSpawn puts uuid.Generate in context: at ~4.2us per spawn the id generator is ~1%
// of it and its mutex ~0.2%, so making uuid lock-free is not worth the complexity.
func BenchmarkSpawn(b *testing.B) {
	sys := newTestSystemTB(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		sys.Spawn(func() IActor { return &sink{} })
	}
}
