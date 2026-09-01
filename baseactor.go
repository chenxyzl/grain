package grain

import (
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/chenxyzl/grain/message"
	"google.golang.org/protobuf/proto"
)

//var _ IActor = (*BaseActor)(nil)

// BaseActor is embedded by every user actor to get Self/GetSystem/Logger/Ask and the
// self-scheduling helpers.
//
// It holds its own reference in a NAMED field rather than embedding ActorRef. The
// embedded form made every user actor implicitly *be* an ActorRef, which had three
// bad consequences: `x.Tell(msg)` read like "tell someone" while actually meaning
// "tell myself"; `&MyActor{}` compiled anywhere an ActorRef was expected, silently
// passing an actor where a reference belonged; and before _init the embedded interface
// was nil, so touching it from a constructor or field initializer nil-panicked with no
// explanation.
//
// Consequence for callers: reference operations now go through Self() —
// `x.Self().Tell(m)`, `x.Self().GetId()` — which says what it does.
type BaseActor struct {
	self ActorRef
	// system is cached from self at _init: every helper needs it, and this avoids an
	// interface hop through self.GetSystem() each time.
	system ISystem
	// logger is built on FIRST USE, not at _init: slog.With measured ~680ns / 360B /
	// 7 allocs, which was ~16% of a spawn's 4.2us and 360B retained for the actor's
	// whole lifetime — 360MB at a million actors, all permanently GC-scanned. Most
	// actors never log, so most now pay nothing.
	//
	// atomic.Pointer rather than a plain field: actor code is single-threaded via the
	// turn, but users do spawn goroutines from handlers and may log from them, and an
	// unsynchronized lazy write there would be a genuine data race. Two racers may both
	// build a logger; they are equivalent, so last-writer-wins is fine. The load is ~1ns.
	logger atomic.Pointer[slog.Logger]
	turn   reentryTurn // owning processor's turn controller, for reentrant Ask
}

//func (x *BaseActor) Started()             {}
//func (x *BaseActor) PreStop()             {}
//func (x *BaseActor) Receive(ctx IContext) {}

func (x *BaseActor) _init(self ActorRef) {
	x.self = self
	x.system = self.GetSystem()
	// logger is deliberately NOT built here — see the field comment.
}
func (x *BaseActor) _bindTurn(t reentryTurn) { x.turn = t }

// mustInited turns "used before the actor was spawned" from an unexplained nil
// dereference into a message that names the mistake.
func (x *BaseActor) mustInited() {
	if x.self == nil {
		panic("grain: BaseActor used before the actor was spawned. Self/GetSystem/Logger/Ask " +
			"are only valid from Started, Receive and PreStop — not from a constructor, a field " +
			"initializer, or the Producer func")
	}
}

// Self returns this actor's own reference. Use it for anything reference-shaped:
// Self().Tell(m), Self().GetId(), Self().GetKind().
func (x *BaseActor) Self() ActorRef {
	x.mustInited()
	return x.self
}

// GetSystem returns the actor system this actor belongs to.
func (x *BaseActor) GetSystem() ISystem {
	x.mustInited()
	return x.system
}

// Logger returns a logger tagged with this actor's reference, derived from the SYSTEM
// logger — so every actor line also carries the system address and node id the system
// logger was built with (system_life.go). It used to derive from slog.Default(), which
// left actor logs with no way to tell which node they came from once several nodes wrote
// to the same collector; the actor ref alone does not say, since a cluster ref is the
// same string on whichever node holds the grain.
//
// Output format changes accordingly: actor lines gain system=... node=... attributes.
func (x *BaseActor) Logger() *slog.Logger {
	x.mustInited()
	if l := x.logger.Load(); l != nil {
		return l
	}
	// x.system.Logger() rather than slog.Default(): resolved on FIRST USE, which is
	// after system.init() replaced the bootstrap logger with the one carrying
	// system+node. (An actor spawned before Start() would fall back to the bootstrap
	// slog.Default() — but such an actor has an empty address and is unroutable
	// anyway.)
	l := x.system.Logger().With("actor", x.self)
	x.logger.Store(l)
	return l
}

// Ask sends msg to target and blocks for a reply of type T. It is reentrant:
// while this actor waits, it yields its execution turn so other messages
// (including a reply chain a->b->a) can be processed, then reacquires the turn
// before returning. The actor stays strictly single-threaded.
//
// T must be written explicitly — it appears only in the result, so it cannot be
// inferred from the arguments:
//
//	reply, err := x.Ask[*pb.HelloReply](target, &pb.HelloAsk{})
//
// It may only be called while the actor is running — from a normal handler. It is
// refused from Started() (reentrancy is off there, so the actor cannot answer
// incoming requests and the Ask may never be satisfiable) and from PreStop()
// (blocking there re-enters the stop path). Either way it sends nothing and returns
// message.CodeAskNotRunning immediately. To Ask at startup, Tell self a message
// from Started() and Ask when handling it:
//
//	func (x *A) Started()            { x.Self().Tell(&pb.Kickoff{}) }
//	func (x *A) Receive(c Context)   { /* case *pb.Kickoff: x.Ask[...](...) */ }
//
// Returns the typed reply, or an *message.ErrCode on failure (called outside the
// running phase, nil target, timeout, actor not found, remote error, reply type
// mismatch, or shutdown while waiting). Runtime failures are returned, not
// panicked. Test for a specific cause with errors.Is against a message.Code —
// errors.Is(err, message.CodeActorNotFound) — or switch on message.CodeOf(err).
func (x *BaseActor) Ask[T proto.Message](target ActorRef, msg proto.Message) (T, *message.ErrCode) {
	return askImpl[T](x.Self(), target, msg, x.turn)
}

// ScheduleSelfOnce Tells this actor msg after delay. msg is delivered as-is, so do not
// mutate it in the meantime — see IScheduler.
func (x *BaseActor) ScheduleSelfOnce(delay time.Duration, msg proto.Message) CancelScheduleFunc {
	return x.GetSystem().GetScheduler().ScheduleOnce(x.Self(), delay, msg)
}

// ScheduleSelfRepeated Tells this actor msg after delay, then every interval until the
// returned func is called.
//
// ⚠️ Every tick delivers the SAME msg instance, so a field this actor writes while
// handling one tick is still set on the next. Schedule a fieldless trigger and build the
// real message in the handler, or proto.Clone before mutating — see IScheduler.
func (x *BaseActor) ScheduleSelfRepeated(delay time.Duration, interval time.Duration, msg proto.Message) CancelScheduleFunc {
	return x.GetSystem().GetScheduler().ScheduleRepeated(x.Self(), delay, interval, msg)
}
