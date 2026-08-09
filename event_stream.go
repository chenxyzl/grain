package grain

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/chenxyzl/grain/al/safemap"
	"github.com/chenxyzl/grain/message"
	"google.golang.org/protobuf/proto"
)

var _ IActor = (*eventStream)(nil)

type eventStream struct {
	BaseActor
	provider          iProvider
	nodeId            uint64
	eventStreamPrefix string
	eventStreamMaps   *safemap.RWMap[string, *safemap.RWMap[uint64, ActorRef]] //eventName:eventStreamId:nodeId-actorId
	sub               map[string]map[string]bool                               // eventName:actorId:_
}

func newEventStream(nodeId uint64, provider iProvider, eventStreamPrefix string) IActor {
	return &eventStream{
		nodeId:            nodeId,
		provider:          provider,
		eventStreamPrefix: eventStreamPrefix,
		eventStreamMaps:   safemap.NewRWMap[string, *safemap.RWMap[uint64, ActorRef]](),
		sub:               make(map[string]map[string]bool)}
}

func (x *eventStream) Started() {
	x.Logger().Info("EventStream started ...")
	//watcher eventStream
	err := x.watchEventStream()
	if err != nil {
		// runtime failure (etcd watch): log and stop self rather than panic
		// (which would only be swallowed by start()'s recover anyway).
		x.Logger().Error("watchEventStream err, stop self", "err", err)
		x.GetSystem().Poison(x.Self())
		return
	}
}

func (x *eventStream) PreStop() {
	x.Logger().Info("eventStream stopped ...")
}

func (x *eventStream) Receive(ctx Context) {
	switch msg := ctx.Message().(type) {
	case *message.Subscribe:
		x.subscribe(ctx, msg)
	case *message.Unsubscribe:
		x.unsubscribe(ctx, msg)
	case *message.BroadcastPublishProtoWrapper:
		x.broadcastPublish(ctx, msg.Message)
	case proto.Message:
		x.onPublish(ctx, msg)
	}
}

func (x *eventStream) subscribe(_ Context, msg *message.Subscribe) {
	if _, ok := x.sub[msg.EventName]; !ok {
		x.sub[msg.EventName] = make(map[string]bool)
		x.registerEventStream(msg.EventName)
		x.Logger().Debug("EventStream subscribe from etcd: ", "eventName", msg.EventName)
	}
	x.sub[msg.EventName][msg.GetActorId()] = true
	x.Logger().Debug("EventStream subscribed", "id", msg.GetActorId(), "eventName", msg.EventName)
}

func (x *eventStream) unsubscribe(_ Context, msg *message.Unsubscribe) {
	if x.sub[msg.EventName] == nil {
		return
	}
	if _, ok := x.sub[msg.EventName][msg.GetActorId()]; !ok {
		return
	}
	//
	delete(x.sub[msg.EventName], msg.GetActorId())
	//
	x.Logger().Debug("EventStream unsubscribe", "id", msg.GetActorId(), "eventName", msg.EventName)
	//
	if len(x.sub[msg.EventName]) == 0 {
		delete(x.sub, msg.EventName)
		x.unregisterEventStream(msg.EventName)
		x.Logger().Debug("EventStream unsubscribe from etcd: ", "eventName", msg.EventName)
	}
}

func (x *eventStream) broadcastPublish(_ Context, msg proto.Message) {
	actors := x.getActorsByEventFromEventStream(msg)
	for _, actorRef := range actors {
		x.GetSystem().getSender().tell(actorRef, msg)
	}
}

func (x *eventStream) onPublish(_ Context, msg proto.Message) {
	eventName := string(proto.MessageName(msg))
	for actorId := range x.sub[eventName] {
		actorRef := newActorRefFromAID(actorId, x.GetSystem())
		x.GetSystem().getSender().tell(actorRef, msg)
	}
}

func (x *eventStream) watchEventStream() error {
	// initial full load + continuous watch, delegated to the provider so this
	// actor stays free of etcd (clientv3/mvccpb) types.
	return x.provider.watchEventStream(x.eventStreamPrefix, func(op watchOp, key string, val []byte) {
		_ = x.parseWatchEventStream(op, key, val)
	})
}

func (x *eventStream) parseWatchEventStream(op watchOp, key string, value []byte) (err error) {
	//"/$clusterName/event_stream/$eventName/$actor_id"
	key = strings.TrimPrefix(key, "/")
	arr := strings.SplitN(key, "/", 4)
	if len(arr) != 4 {
		return fmt.Errorf("invalid eventStream, len err, key:%v", key)
	}
	eventName := arr[2]
	nodeId, err := strconv.Atoi(arr[3])
	if err != nil {
		return fmt.Errorf("invalid eventStream, convert to nodeId err, key:%v, err:%v", key, err)
	}

	actors, b := x.eventStreamMaps.Get(eventName)
	if op == watchDelete {
		if b && actors != nil {
			actors.Delete(uint64(nodeId))
		}
		x.Logger().Warn("event stream key delete, success", "key", key)
		return nil
	} else {
		if actors == nil {
			actors = safemap.NewRWMap[uint64, ActorRef]()
			x.eventStreamMaps.Set(eventName, actors)
		}
		actorRef := newActorRefFromAID(string(value), x.GetSystem())
		if actorRef == nil {
			return fmt.Errorf("invalid eventStream, id to actorRef err, key:%v", key)
		}
		actors.Set(uint64(nodeId), actorRef)
		x.Logger().Warn("event stream key add, success", "key", key)
	}
	return nil
}

func (x *eventStream) getEventNamePath(eventName string) string {
	str := strings.ReplaceAll(x.eventStreamPrefix+"/"+eventName+"/"+strconv.Itoa(int(x.nodeId)), "//", "/")
	return str
}

func (x *eventStream) registerEventStream(eventName string) {
	path := x.getEventNamePath(eventName)
	if err := x.provider.registerEventStream(path, x.Self().GetId()); err != nil {
		x.Logger().Error("register eventStream error", "path", path, "eventName", eventName, "err", err)
		return
	}
	//change local
	actors, _ := x.eventStreamMaps.Get(eventName)
	if actors == nil {
		actors = safemap.NewRWMap[uint64, ActorRef]()
		x.eventStreamMaps.Set(eventName, actors)
	}
	actors.Set(x.nodeId, x.Self())
}
func (x *eventStream) unregisterEventStream(eventName string) {
	path := x.getEventNamePath(eventName)
	if err := x.provider.unregisterEventStream(path); err != nil {
		x.Logger().Error("unregister eventStream error", "path", path, "eventName", eventName, "err", err)
		return
	}
	//change local
	actors, _ := x.eventStreamMaps.Get(eventName)
	if actors == nil {
		return
	}
	actors.Delete(x.nodeId)
}
func (x *eventStream) getActorsByEventFromEventStream(event proto.Message) []ActorRef {
	eventName := string(proto.MessageName(event))
	actors, b := x.eventStreamMaps.Get(eventName)
	if !b {
		return nil
	}
	var ret []ActorRef
	actors.Range(func(nodeId uint64, ref ActorRef) bool {
		ret = append(ret, ref)
		return true
	})
	return ret
}
