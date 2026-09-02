package grain

import (
	"fmt"
	"sync"
	"time"

	"github.com/chenxyzl/grain/message"
	"google.golang.org/protobuf/proto"
)

// NoReentryAsk sends req to target and blocks for a reply of type T WITHOUT releasing any actor
// execution turn, so it must NOT be called from inside an actor handler — use the reentrant
// BaseActor.Ask[T] there. This is the entry point for non-actor code: main, tests, http
// handlers. A nil *message.ErrCode means success; otherwise match the cause with
// errors.Is(err, message.CodeActorNotFound) or switch on message.CodeOf(err).
func NoReentryAsk[T proto.Message](target ActorRef, req proto.Message) (T, *message.ErrCode) {
	return askImpl[T](nil, target, req, nil)
}

// askImpl is the shared body of BaseActor.Ask and NoReentryAsk: allocate a correlation id,
// register the reply channel, send with a replyRef as sender, then block for the reply. A non-nil
// turn is released before the send and reacquired after — that is BaseActor.Ask's reentrancy.
// asker is used only in the nil-target diagnostic and may be nil.
func askImpl[T proto.Message](asker ActorRef, target ActorRef, req proto.Message, turn reentryTurn) (T, *message.ErrCode) {
	var null T
	// A blocking Ask is permitted only while the actor is running, refused here at the earliest
	// decidable point, and nothing is sent. Started(): reentrancy is off, so the actor cannot
	// answer incoming requests and an Ask depending on one would silently wait out askTimeout.
	// PreStop(): yielding the turn lets a successor drainer re-enter doStop -> stop() while
	// procStatus is still `running`, running PreStop twice. An allow-list (isStarted), so any
	// lifecycle phase added later is refused by default.
	if turn != nil && !turn.isStarted() {
		return null, errAskNotRunning
	}
	if target == nil {
		return null, message.WithErr(fmt.Sprintf("ask target is nil, sender:%v", asker))
	}
	sys := target.GetSystem()
	snId := sys.nextSnId()
	ch := sys.registerAsk(snId)
	defer sys.cancelAsk(snId) // idempotent: no-op if a reply already Popped it
	// Yield BEFORE sending: the send may block on a full mailbox, and for a self-ask or a full
	// a->b->a cycle only the successor drainer yieldTurn spawns can free space. It also breaks
	// the a->b->a reply deadlock.
	var ds *drainState
	if turn != nil {
		ds = turn.yieldTurn()
	}
	sys.getSender().tellWithSender(target, req, newReplyRef(snId, sys.getAddr(), sys), snId)
	v, err := awaitReply[T](ch, sys.getConfig().askTimeout)
	if turn != nil {
		turn.resumeTurn(ds)
	}
	return v, err
}

// askTimerPool reuses per-Ask timeout timers, avoiding the ~3 allocations time.NewTimer costs
// per Ask. A timer's lifetime is fully scoped to one awaitReply call (Reset on borrow, Stop +
// return on release), so concurrent and nested/reentrant Asks each hold their own.
var askTimerPool = sync.Pool{
	New: func() any {
		t := time.NewTimer(time.Hour)
		t.Stop()
		return t
	},
}

// awaitReply blocks for a reply or the timeout, then decodes it into T. Nil ErrCode = success.
func awaitReply[T proto.Message](ch chan proto.Message, timeout time.Duration) (T, *message.ErrCode) {
	var null T
	timer := askTimerPool.Get().(*time.Timer)
	timer.Reset(timeout)
	defer func() {
		// Go 1.23+: Stop plus Reset-on-next-borrow cleans a fired-but-unread C, so no manual drain
		timer.Stop()
		askTimerPool.Put(timer)
	}()
	select {
	case resp := <-ch:
		// The framework sentinels are matched BEFORE T on purpose: both are themselves
		// proto.Messages, so a `case T` placed first would hand them back as successful replies
		// whenever T is an interface (Ask[proto.Message]). Consequence: Ask[*message.ErrCode]
		// reports an error rather than returning the ErrCode as a value.
		switch msg := resp.(type) {
		case *message.Poison:
			return null, message.WithErr("ask reply poisoned")
		case *message.ErrCode:
			return null, msg
		case T:
			return msg, nil
		case error:
			return null, message.WithErr(msg.Error())
		default:
			return null, message.WithErr(fmt.Sprintf("msg type err, need:%v, now:%v",
				proto.MessageName(null), proto.MessageName(msg)))
		}
	case <-timer.C:
		return null, message.WithErr("ask reply timeout")
	}
}
