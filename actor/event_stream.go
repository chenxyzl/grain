package actor

import "google.golang.org/protobuf/proto"

var _ IActor = (*EventStream)(nil)

type EventStream struct {
	BaseActor
	sub map[string]map[string]bool // eventName:actorId:_
}

func newEventStream() *EventStream {
	return &EventStream{sub: make(map[string]map[string]bool)}
}

func (x *EventStream) Started() {
	x.Logger().Info("EventStream started ...")
}

func (x *EventStream) PreStop() {
	x.Logger().Info("EventStream stopped ...")
}

func (x *EventStream) Receive(ctx IContext) {
	switch msg := ctx.Message().(type) {
	case *Subscribe:
		if _, ok := x.sub[msg.EventName]; !ok {
			x.sub[msg.EventName] = make(map[string]bool)
		}
		x.sub[msg.EventName][msg.GetSelf().GetId()] = true
	case *Unsubscribe:
		if x.sub[msg.EventName] != nil && x.sub[msg.EventName][msg.GetSelf().GetId()] == true {
			delete(x.sub[msg.EventName], msg.GetSelf().GetId())
		}
	case proto.Message:
		eventName := string(proto.MessageName(msg))
		for ref := range x.sub[eventName] {
			actorRef := newActorRefFromId(ref)
			x.system.send(actorRef, msg, ctx.GetMsgSnId())
		}
	}
}
