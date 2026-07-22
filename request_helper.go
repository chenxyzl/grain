package grain

import (
	"github.com/chenxyzl/grain/message"
	"google.golang.org/protobuf/proto"
)

// NoReentryAsk mean's not allowed re-entry
// wanted system.NoReentryAsk[T proto.Message](target ActorRef, req proto.Message) T
// but golang not support
func NoReentryAsk[T proto.Message](target ActorRef, req proto.Message) (T, *message.ErrCode) {
	sys := target.GetSystem()
	msgSnId := target.GetSystem().nextSnId()
	reqTimeout := sys.getConfig().askTimeout
	//
	reply := newProcessorReplay[T](sys, reqTimeout)
	//
	sys.getSender().tellWithSender(target, req, reply.self(), msgSnId)
	//
	return reply.Result()
}
