package grain

import "google.golang.org/protobuf/proto"

// NoReentryRequest mean's not allowed re-entry
// wanted system.NoReentryRequest[T proto.Message](target ActorRef, req proto.Message) T
// but golang not support
func NoReentryRequest[T proto.Message](target ActorRef, req proto.Message) (T, error) {
	sys := target.GetSystem()
	msgSnId := target.GetSystem().getGenRequestId().genRequestId()
	reqTimeout := sys.getConfig().requestTimeout
	//
	reply := newProcessorReplay[T](sys, reqTimeout)
	//
	sys.getSender().tellWithSender(target, req, reply.self(), msgSnId)
	//
	return reply.Result()
}
