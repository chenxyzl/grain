package grain

import (
	"fmt"
	"sync"
	"time"

	"github.com/chenxyzl/grain/message"
	"google.golang.org/protobuf/proto"
)

// NoReentryAsk mean's not allowed re-entry
// wanted system.NoReentryAsk[T proto.Message](target ActorRef, req proto.Message) T
// but golang not support
func NoReentryAsk[T proto.Message](target ActorRef, req proto.Message) (T, *message.ErrCode) {
	sys := target.GetSystem()
	snId := sys.nextSnId()
	ch := sys.registerAsk(snId)
	defer sys.cancelAsk(snId)
	//
	sys.getSender().tellWithSender(target, req, newReplyRef(snId, sys.getAddr(), sys), snId)
	//
	return awaitReply[T](ch, sys.getConfig().askTimeout)
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
		switch msg := resp.(type) {
		case T:
			return msg, nil
		case *message.Poison:
			return null, message.WithErr("ask reply poisoned")
		case *message.ErrCode:
			return null, msg
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
