package grain

import (
	"google.golang.org/protobuf/proto"
)

var _ Context = (*contextImpl)(nil)

type contextImpl struct {
	target     ActorRef
	sender     ActorRef
	msgSnId    uint64
	message    proto.Message
	senderFunc iSender
}

func (x *contextImpl) Reply(message proto.Message) {
	// Only a reply owned by this sending system can bypass routing. Cross-system
	// forwarded requests, remote replies and custom senders retain their normal path.
	if reply, ok := x.sender.(*replyRef); ok {
		if sys, ok := x.senderFunc.(*system); ok && reply.system == sys && reply.addr == sys.getAddr() {
			sys.deliverReply(reply.snId, message)
			return
		}
	}
	x.senderFunc.tellWithSender(x.Sender(), message, x.Target(), x.msgSnId)
}

func newContext(target ActorRef, sender ActorRef, message proto.Message, msgSnId uint64, senderFunc iSender) Context {
	return &contextImpl{
		target:     target,
		sender:     sender,
		msgSnId:    msgSnId,
		message:    message,
		senderFunc: senderFunc,
	}
}

func (x *contextImpl) Target() ActorRef {
	return x.target
}

// Sender may be nil — a plain Tell carries no sender. No nil normalization here: the guard that
// used to sit in this function was a no-op, because returning a literal nil and returning x.sender
// when x.sender is nil produce the same value, and it did not flatten a non-nil interface holding
// a nil pointer either.
func (x *contextImpl) Sender() ActorRef {
	return x.sender
}

func (x *contextImpl) Message() proto.Message {
	return x.message
}

func (x *contextImpl) GetMsgSnId() uint64 {
	return x.msgSnId
}

// Forward re-sends this message to target as-is, keeping the original sender and msgSnId so
// target replies to the original sender. One-shot, not a persistent redirect rule.
func (x *contextImpl) Forward(target ActorRef) {
	x.senderFunc.tellWithSender(target, x.message, x.sender, x.msgSnId)
}
