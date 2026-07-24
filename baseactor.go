package grain

import (
	"fmt"
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

func (x *BaseActor) Send(target ActorRef, msg proto.Message) {
	if target == nil {
		x.Logger().Error("send target is nil", "id", x.Self(), "msgName", proto.MessageName(msg), "msg", msg)
		return
	}
	x.GetSystem().getSender().tell(target, msg)
}

// Ask sends msg to target and blocks for the reply. It is reentrant: while this
// actor waits, it yields its execution turn so other messages (including a
// reply chain a->b->a) can be processed, then reacquires the turn before
// returning. The actor stays strictly single-threaded.
//
// Returns the reply, or an *message.ErrCode on failure (nil target, timeout,
// remote error, type mismatch). Runtime failures are returned, not panicked.
func (x *BaseActor) Ask(target ActorRef, msg proto.Message) (proto.Message, *message.ErrCode) {
	if target == nil {
		return nil, message.WithErr(fmt.Sprintf("ask target is nil, sender:%v", x.Self()))
	}
	//
	sys := target.GetSystem()
	snId := sys.nextSnId()
	ch := sys.registerAsk(snId)
	defer sys.cancelAsk(snId) // idempotent: no-op if a reply already Popped it
	// Yield the turn BEFORE sending: the send may block on a full mailbox, and if
	// the target is this actor itself (self-ask) or a full a->b->a cycle, only a
	// successor drainer (spawned by yieldTurn) can free space. Yielding first lets
	// that successor drain while we send, avoiding a self-deadlock. Yielding also
	// breaks the a->b->a reply deadlock.
	ds := x.turn.yieldTurn()
	sys.getSender().tellWithSender(target, msg, newReplyRef(snId, sys.getAddr(), sys), snId)
	v, err := awaitReply[proto.Message](ch, sys.getConfig().askTimeout)
	x.turn.resumeTurn(ds)
	return v, err
}

func (x *BaseActor) ScheduleSelfOnce(delay time.Duration, msg proto.Message) CancelScheduleFunc {
	return x.GetSystem().GetScheduler().ScheduleOnce(x.Self(), delay, msg)
}

func (x *BaseActor) ScheduleSelfRepeated(delay time.Duration, interval time.Duration, msg proto.Message) CancelScheduleFunc {
	return x.GetSystem().GetScheduler().ScheduleRepeated(x.Self(), delay, interval, msg)
}
