package grain

import (
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/chenxyzl/grain/message"
	"google.golang.org/protobuf/proto"
)

//var _ IActor = (*BaseActor)(nil)

// BaseActor is embedded by every user actor for Self/GetSystem/Logger/Ask and the self-scheduling
// helpers. Its reference is a named field, not an embedded ActorRef: use x.Self().Tell(m) etc.
type BaseActor struct {
	self ActorRef
	// cached from self at _init: every helper needs it, and it saves an interface hop
	system ISystem
	// Built on FIRST USE: slog.With is ~680ns/360B, and the 360B would stay live per actor for
	// life, while most actors never log. atomic.Pointer, not a plain field, because handlers do
	// spawn goroutines that log and an unsynchronized lazy write there is a genuine race; two
	// racers build equivalent loggers, so last-writer-wins is fine.
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

// mustInited turns "used before the actor was spawned" from a bare nil dereference into a
// message that names the mistake.
func (x *BaseActor) mustInited() {
	if x.self == nil {
		panic("grain: BaseActor used before the actor was spawned. Self/GetSystem/Logger/Ask " +
			"are only valid from Started, Receive and PreStop — not from a constructor, a field " +
			"initializer, or the Producer func")
	}
}

// Self returns this actor's own reference: Self().Tell(m), Self().GetId(), Self().GetKind().
func (x *BaseActor) Self() ActorRef {
	x.mustInited()
	return x.self
}

// GetSystem returns the actor system this actor belongs to.
func (x *BaseActor) GetSystem() ISystem {
	x.mustInited()
	return x.system
}

// Logger returns a logger tagged with this actor's ref and derived from the SYSTEM logger, so
// actor lines also carry system address and node id — a cluster ref alone does not name the node.
func (x *BaseActor) Logger() *slog.Logger {
	x.mustInited()
	if l := x.logger.Load(); l != nil {
		return l
	}
	// Resolved on first use, hence after system.init() swapped in the system+node logger. An actor
	// spawned before Start() falls back to bootstrap slog.Default(), but is unroutable anyway.
	l := x.system.Logger().With("actor", x.self)
	x.logger.Store(l)
	return l
}

// Ask sends msg to target and blocks for a reply of type T, which must be written explicitly
// since it appears only in the result: x.Ask[*pb.HelloReply](target, &pb.HelloAsk{}). Reentrant —
// while waiting it yields its execution turn so other messages (including a reply chain a->b->a)
// still get processed, then reacquires it; the actor stays strictly single-threaded.
//
// Only valid from a normal handler. Refused from Started() (reentrancy is off there, so the actor
// cannot answer incoming requests and the Ask may never be satisfiable) and from PreStop()
// (blocking re-enters the stop path): nothing is sent, message.CodeAskNotRunning returns at once.
// To Ask at startup, Tell self from Started() and Ask while handling that message.
//
// Failures are RETURNED as an *message.ErrCode, never panicked (wrong phase, nil target, timeout,
// actor not found, remote error, reply type mismatch, shutdown while waiting). Match a cause with
// errors.Is(err, message.CodeActorNotFound) or switch on message.CodeOf(err).
func (x *BaseActor) Ask[T proto.Message](target ActorRef, msg proto.Message) (T, *message.ErrCode) {
	return askImpl[T](x.Self(), target, msg, x.turn)
}

// ScheduleSelfOnce Tells this actor msg after delay. msg is delivered as-is, so do not mutate it
// in the meantime — see IScheduler.
func (x *BaseActor) ScheduleSelfOnce(delay time.Duration, msg proto.Message) CancelScheduleFunc {
	return x.GetSystem().GetScheduler().ScheduleOnce(x.Self(), delay, msg)
}

// ScheduleSelfRepeated Tells this actor msg after delay, then every interval until the returned
// func is called. ⚠️ Every tick delivers the SAME msg instance, so a field written while handling
// one tick is still set on the next: schedule a fieldless trigger and build the real message in
// the handler, or proto.Clone before mutating. See IScheduler.
func (x *BaseActor) ScheduleSelfRepeated(delay time.Duration, interval time.Duration, msg proto.Message) CancelScheduleFunc {
	return x.GetSystem().GetScheduler().ScheduleRepeated(x.Self(), delay, interval, msg)
}
