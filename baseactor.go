package grain

import (
	"log/slog"
	"time"

	"github.com/chenxyzl/grain/message"
	"google.golang.org/protobuf/proto"
)

//var _ IActor = (*BaseActor)(nil)

type BaseActor struct {
	ActorRef
	logger *slog.Logger
	turn   reentryTurn // owning processor's turn controller, for reentrant Ask
}

//func (x *BaseActor) Started()             {}
//func (x *BaseActor) PreStop()             {}
//func (x *BaseActor) Receive(ctx IContext) {}

func (x *BaseActor) _init(self ActorRef) {
	x.ActorRef = self
	x.logger = slog.With("actor", x.Self()) //warning: slog.With performance too slow
}
func (x *BaseActor) _bindTurn(t reentryTurn) { x.turn = t }

func (x *BaseActor) Self() ActorRef       { return x.ActorRef }
func (x *BaseActor) Logger() *slog.Logger { return x.logger }

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
// Returns the typed reply, or an *message.ErrCode on failure (nil target,
// timeout, actor not found, remote error, reply type mismatch, or shutdown while
// waiting). Runtime failures are returned, not panicked.
func (x *BaseActor) Ask[T proto.Message](target ActorRef, msg proto.Message) (T, *message.ErrCode) {
	return askImpl[T](x.Self(), target, msg, x.turn)
}

func (x *BaseActor) ScheduleSelfOnce(delay time.Duration, msg proto.Message) CancelScheduleFunc {
	return x.GetSystem().GetScheduler().ScheduleOnce(x.Self(), delay, msg)
}

func (x *BaseActor) ScheduleSelfRepeated(delay time.Duration, interval time.Duration, msg proto.Message) CancelScheduleFunc {
	return x.GetSystem().GetScheduler().ScheduleRepeated(x.Self(), delay, interval, msg)
}
