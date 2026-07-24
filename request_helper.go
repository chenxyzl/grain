package grain

import (
	"fmt"
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

// awaitReply blocks on the correlation channel until a reply arrives or timeout,
// then decodes it into T. Shared by BaseActor.Ask and NoReentryAsk. A nil ErrCode
// means success.
func awaitReply[T proto.Message](ch chan proto.Message, timeout time.Duration) (T, *message.ErrCode) {
	var null T
	timer := time.NewTimer(timeout)
	defer timer.Stop()
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
				null.ProtoReflect().Descriptor().FullName(), msg.ProtoReflect().Descriptor().FullName()))
		}
	case <-timer.C:
		return null, message.WithErr("ask reply timeout")
	}
}
