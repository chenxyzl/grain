package grain

import (
	"fmt"
	"sync"
	"time"

	"github.com/chenxyzl/grain/message"
	"google.golang.org/protobuf/proto"
)

// NoReentryAsk sends req to target and blocks for a reply of type T WITHOUT
// releasing any actor execution turn, so it must not be called from inside an
// actor handler (use BaseActor.Ask[T] there, which is reentrant). It is the entry
// point for non-actor code: main, tests, http handlers.
//
// A nil *message.ErrCode means success. On failure, match the cause with errors.Is
// against a message.Code — errors.Is(err, message.CodeActorNotFound) — or switch on
// message.CodeOf(err).
func NoReentryAsk[T proto.Message](target ActorRef, req proto.Message) (T, *message.ErrCode) {
	return askImpl[T](nil, target, req, nil)
}

// askImpl is the shared body of BaseActor.Ask and NoReentryAsk: allocate a
// correlation id, register the reply channel, send with a replyRef as the sender,
// then block until the reply arrives, times out, or the system shuts down.
//
// When turn is non-nil the caller's actor execution turn is released before the
// send and reacquired after the reply — that is what makes BaseActor.Ask
// reentrant. NoReentryAsk passes nil and simply blocks.
//
// asker is used only for the nil-target diagnostic and may be nil.
func askImpl[T proto.Message](asker ActorRef, target ActorRef, req proto.Message, turn reentryTurn) (T, *message.ErrCode) {
	var null T
	// A blocking Ask is only permitted while the actor is running — after Started()
	// completed and before PreStop() began — and is refused here at the call site,
	// the earliest point at which this is decidable.
	//
	// Started(): reentrancy is off (a handler must not run against half-initialized
	// state), so the actor cannot answer any incoming request in that window; an Ask
	// whose reply depends on that would silently wait out askTimeout. Whether a
	// *given* Ask depends on it is NOT predictable here, so the phase is disallowed
	// outright rather than guessed at.
	//
	// PreStop(): yielding the turn there lets a successor drainer re-enter
	// doStop -> stop() while procStatus is still `running`, which used to run PreStop
	// a second time (see lifeStopping).
	//
	// Phrased as an allow-list (isStarted) so any lifecycle phase added later is
	// refused by default instead of silently permitting a blocking Ask. Nothing is
	// sent, so the target never sees the request.
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
	// Yield the turn BEFORE sending: the send may block on a full mailbox, and if
	// the target is this actor itself (self-ask) or a full a->b->a cycle, only a
	// successor drainer (spawned by yieldTurn) can free space. Yielding first lets
	// that successor drain while we send, avoiding a self-deadlock. Yielding also
	// breaks the a->b->a reply deadlock.
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

// askTimerPool reuses per-Ask timeout timers. The timer's lifetime is fully
// scoped to one awaitReply call (Reset on borrow, Stop + return on release), so
// pooling is safe and avoids the ~3 allocations time.NewTimer costs per Ask.
// Handles concurrent and nested/reentrant Asks: each in-flight call holds its
// own borrowed timer.
var askTimerPool = sync.Pool{
	New: func() any {
		t := time.NewTimer(time.Hour)
		t.Stop()
		return t
	},
}

// awaitReply blocks on the correlation channel until a reply arrives or timeout,
// then decodes it into T. Shared by BaseActor.Ask and NoReentryAsk. A nil ErrCode
// means success.
func awaitReply[T proto.Message](ch chan proto.Message, timeout time.Duration) (T, *message.ErrCode) {
	var null T
	timer := askTimerPool.Get().(*time.Timer)
	timer.Reset(timeout)
	defer func() {
		// Go 1.23+: Stop then Reset-on-next-borrow cleans a fired-but-unread C,
		// so no manual drain is needed before returning the timer to the pool.
		timer.Stop()
		askTimerPool.Put(timer)
	}()
	select {
	case resp := <-ch:
		// The framework sentinels are matched BEFORE T on purpose. Both are
		// themselves proto.Messages, so a `case T` placed first would match them
		// whenever T is an interface (Ask[proto.Message]) and hand a failure back
		// as a successful reply. Matching them first makes the error semantics hold
		// for every T. Consequence: Ask[*message.ErrCode] reports an error instead
		// of returning the ErrCode as a value — asking for the error type itself is
		// not a supported use.
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
