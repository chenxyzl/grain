package grain

import (
	"github.com/chenxyzl/grain/remote"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// msgNameInitialize is the wire name of the activation signal, taken from the message itself so a
// proto rename cannot silently reopen the hole below. Compared as a string before any
// unmarshalling, so a hostile envelope costs one comparison.
var msgNameInitialize = string(proto.MessageName(msgInitialize))

func (x *system) RecvEnvelope(envelope *remote.Envelope) {
	// Entry point for data from another node, so treat it as untrusted: nil-safe getters only,
	// and reject nil rather than panic on the grpc handler goroutine, which has no recover.
	if envelope == nil {
		x.Logger().Error("recvEnvelope, nil envelope")
		return
	}
	msgName := envelope.GetMsgName()
	// Initialize is the framework's own activation signal, and it is a normally registered proto,
	// so without this check any peer could name it in an envelope and have invoke() re-run start()
	// on a live actor: Started() a second time on a running actor, or — for a cluster grain — a
	// setTxn against our own key, which fails and then tears the grain down via its own valid lock.
	// Poison is deliberately NOT rejected: routing it is exactly how a remote stop works.
	if msgName == msgNameInitialize {
		x.Logger().Error("recvEnvelope, refusing a remote framework control message",
			"actor", envelope.GetTarget(), "msgName", msgName)
		return
	}
	typ, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(msgName))
	if err != nil {
		x.Logger().Error("recvEnvelope, unregister msg type", "actor", envelope.GetTarget(), "msgName", msgName, "err", err)
		return
	}
	bodyMsg := typ.New().Interface()
	err = proto.Unmarshal(envelope.GetContent(), bodyMsg)
	if err != nil {
		x.Logger().Error("recvEnvelope, msg unmarshal err", "actor", envelope.GetTarget(), "msgName", msgName, "err", err)
		return
	}
	// A target is mandatory; without one there is nothing to route to
	if envelope.GetTarget() == "" {
		x.Logger().Error("recvEnvelope, empty target", "msgName", msgName)
		return
	}
	// A sender is optional, and "" must stay nil, or `ctx.Sender() != nil` is a false positive
	var sender ActorRef
	if s := envelope.GetSender(); s != "" {
		sender = newActorRefFromAID(s, x)
	}
	var target = newActorRefFromAID(envelope.GetTarget(), x)
	x.tellWithSender(target, bodyMsg, sender, envelope.GetMsgSnId())
}
