package grain

import (
	"fmt"
	"sync"
	"time"

	"github.com/chenxyzl/grain/message"
	"google.golang.org/protobuf/proto"
)

// NoReentryAsk sends req to target and blocks for a reply of type T WITHOUT releasing any actor
// execution permission, so it must NOT be called from inside an actor handler — use the reentrant
// BaseActor.Ask[T] there. This is the entry point for non-actor code: main, tests, http
// handlers. A nil *message.ErrCode means success; otherwise match the cause with
// errors.Is(err, message.CodeActorNotFound) or switch on message.CodeOf(err).
func NoReentryAsk[T proto.Message](target ActorRef, req proto.Message) (T, *message.ErrCode) {
	return askImpl[T](nil, target, req, nil)
}

// askImpl pairs suspension with restoration before returning or unwinding into
// user defers. Pending cleanup precedes resume, which can wait for another handler.
func askImpl[T proto.Message](asker ActorRef, target ActorRef, req proto.Message, call *execution) (T, *message.ErrCode) {
	var zero T
	if target == nil {
		return zero, message.WithErr(fmt.Sprintf("ask target is nil, sender:%v", asker))
	}
	sys := target.GetSystem()
	requestID := sys.nextSnId()
	reply := sys.registerAsk(requestID)
	if reply == nil {
		return zero, errSystemStopping
	}
	if call != nil {
		call.suspend()
		defer call.resume()
	}
	defer sys.cancelAsk(requestID)
	sys.getSender().tellWithSender(target, req, newReplyRef(requestID, sys.getAddr(), sys), requestID)
	return awaitReply[T](reply, sys.getConfig().askTimeout)
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
