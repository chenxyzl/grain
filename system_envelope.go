package grain

import (
	"github.com/chenxyzl/grain/remote"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func (x *system) RecvEnvelope(envelope *remote.Envelope) {
	// This is the entry point for data from another node, so treat it as untrusted:
	// use the nil-safe getters throughout, and reject a nil envelope rather than
	// dereferencing it (a nil here used to panic and, being on a grpc handler
	// goroutine with no recover, took the whole process down).
	if envelope == nil {
		x.Logger().Error("recvEnvelope, nil envelope")
		return
	}
	typ, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(envelope.GetMsgName()))
	if err != nil {
		x.Logger().Error("recvEnvelope, unregister msg type", "actor", envelope.GetTarget(), "msgName", envelope.GetMsgName(), "err", err)
		return
	}
	//new body proto message
	bodyMsg := typ.New().Interface()
	err = proto.Unmarshal(envelope.GetContent(), bodyMsg)
	if err != nil {
		x.Logger().Error("recvEnvelope, msg unmarshal err", "actor", envelope.GetTarget(), "msgName", envelope.GetMsgName(), "err", err)
		return
	}
	// A target is mandatory; without one there is nothing to route to
	if envelope.GetTarget() == "" {
		x.Logger().Error("recvEnvelope, empty target", "msgName", envelope.GetMsgName())
		return
	}
	// A sender is optional; if the sender is nil, the receiver sees a nil sender and cannot reply. This
	var sender ActorRef
	if s := envelope.GetSender(); s != "" {
		sender = newActorRefFromAID(s, x)
	}
	var target = newActorRefFromAID(envelope.GetTarget(), x)
	//build ctx
	x.tellWithSender(target, bodyMsg, sender, envelope.GetMsgSnId())
}
