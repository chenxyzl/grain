package grain

import (
	"testing"

	"github.com/chenxyzl/grain/message"
)

// Isolate routing to an already registered peer stream, excluding serialization,
// mailbox processing and network I/O. Both variants retain the registry lookup.
type benchmarkStreamSink struct {
	ref  ActorRef
	last Context
}

func (p *benchmarkStreamSink) self() ActorRef   { return p.ref }
func (*benchmarkStreamSink) opts() *tOpts       { return nil }
func (*benchmarkStreamSink) init()              {}
func (*benchmarkStreamSink) poison()            {}
func (p *benchmarkStreamSink) send(ctx Context) { p.last = ctx }

func BenchmarkStreamRouting(b *testing.B) {
	for _, cached := range []bool{false, true} {
		name := "rebuild-reference"
		if cached {
			name = "cached-reference"
		}
		b.Run(name, func(b *testing.B) {
			sys := newTestSystemTB(b)
			peer := "127.0.0.1:34567"
			stream := newDirectActorRef(defaultWriteStreamKind, peer, sys.addr, sys)
			sys.registry.processes.Set(stream.GetId(), &benchmarkStreamSink{ref: stream})
			target := newDirectActorRef("player", "123456789", peer, sys)
			msg := &message.Subscribe{}
			sys.routing.resolve(sys, target) // exclude cache population from steady-state timing
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if cached {
					sys.sendToRemote(sys.routing.resolve(sys, target), target, msg, nil, 1)
				} else {
					ref := newDirectActorRef(defaultWriteStreamKind, peer, sys.addr, sys)
					sys.registry.lookup(ref.GetId()).send(newContext(target, nil, msg, 1, sys))
				}
			}
		})
	}
}

// Resolver-only measurements separate address/cache work from registry and Context allocation.
func BenchmarkRoutingResolver(b *testing.B) {
	for _, kind := range []string{"direct", "cluster"} {
		b.Run(kind, func(b *testing.B) {
			sys := newTestSystemTB(b)
			sys.clusterProvider = &fakeProvider{nodes: []tNodeState{{NodeId: 1, Address: "peer:9000", Kinds: []string{"player"}}}}
			var target ActorRef
			if kind == "direct" {
				target = newDirectActorRef("player", "one", "peer:9000", sys)
			} else {
				target = newClusterActorRef("player", "one", sys)
			}
			sys.routing.resolve(sys, target)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if sys.routing.resolve(sys, target).stream == nil {
					b.Fatal("missing stream")
				}
			}
		})
	}
}
