package actor

import (
	"context"
	"errors"
	"fmt"
	"github.com/chenxyzl/grain/utils/al/safemap"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/protobuf/proto"
	"strconv"
	"strings"
)

var _ IActor = (*EventStream)(nil)

type EventStream struct {
	BaseActor
	client            *clientv3.Client
	leaseId           clientv3.LeaseID
	nodeId            uint64
	eventStreamPrefix string
	//eventName:eventStreamId:nodeId-actorId
	eventStreamMaps *safemap.SafeMap[string, *safemap.SafeMap[uint64, *ActorRef]]
	//eventName:localActorRef
	sub map[string]map[string]bool // eventName:actorId:_
}

func newEventStream(nodeId uint64, client *clientv3.Client, leaseId clientv3.LeaseID, eventStreamPrefix string) *EventStream {
	return &EventStream{
		nodeId:            nodeId,
		client:            client,
		leaseId:           leaseId,
		eventStreamPrefix: eventStreamPrefix,
		eventStreamMaps:   safemap.NewM[string, *safemap.SafeMap[uint64, *ActorRef]](),
		sub:               make(map[string]map[string]bool)}
}

func (x *EventStream) Started() {
	x.Logger().Info("EventStream started ...")
	//watcher eventStream
	err := x.watchEventStream()
	if err != nil {
		x.Logger().Error("watchEventStream err: ", err)
		panic(err)
	}
}

func (x *EventStream) PreStop() {
	x.Logger().Info("EventStream stopped ...")
}

func (x *EventStream) Receive(ctx IContext) {
	switch msg := ctx.Message().(type) {
	case *Subscribe:
		x.subscribe(ctx, msg)
	case *Unsubscribe:
		x.unsubscribe(ctx, msg)
	case *publishWrapper:
		x.publishWrapper(ctx, msg.Message)
	case proto.Message:
		x.publish(ctx, msg)
	}
}

func (x *EventStream) subscribe(ctx IContext, msg *Subscribe) {
	if _, ok := x.sub[msg.EventName]; !ok {
		x.sub[msg.EventName] = make(map[string]bool)
		x.registerEventStream(msg.EventName)
		x.Logger().Debug("EventStream subscribe from etcd: ", "eventName", msg.EventName)
	}
	x.sub[msg.EventName][msg.GetSelf().GetId()] = true
	x.Logger().Debug("EventStream subscribed", "id", msg.GetSelf(), "eventName", msg.EventName)
}

func (x *EventStream) unsubscribe(ctx IContext, msg *Unsubscribe) {
	if x.sub[msg.EventName] == nil {
		return
	}
	if _, ok := x.sub[msg.EventName][msg.GetSelf().GetId()]; !ok {
		return
	}
	//
	delete(x.sub[msg.EventName], msg.GetSelf().GetId())
	//
	x.Logger().Debug("EventStream unsubscribe", "id", msg.GetSelf(), "eventName", msg.EventName)
	//
	if len(x.sub[msg.EventName]) == 0 {
		delete(x.sub, msg.EventName)
		x.unregisterEventStream(msg.EventName)
		x.Logger().Debug("EventStream unsubscribe from etcd: ", "eventName", msg.EventName)
	}
}

func (x *EventStream) publishWrapper(ctx IContext, msg proto.Message) {
	actors := x.getActorsByEventFromEventStream(msg)
	for _, actorRef := range actors {
		x.system.sendWithoutSender(actorRef, msg)
	}
}

func (x *EventStream) publish(ctx IContext, msg proto.Message) {
	eventName := string(proto.MessageName(msg))
	for ref := range x.sub[eventName] {
		actorRef := newActorRefFromId(ref)
		x.system.sendWithoutSender(actorRef, msg)
	}
}

func (x *EventStream) watchEventStream() error {
	//first
	rsp, err := x.client.Get(context.Background(), x.eventStreamPrefix, clientv3.WithPrefix())
	if err != nil {
		return errors.Join(err, errors.New("first load eventStream  err"))
	}
	for _, kv := range rsp.Kvs {
		err = x.parseWatchEventStream(mvccpb.PUT, string(kv.Key), kv.Value)
		if err != nil {
			return err
		}
	}
	//real watch
	wch := x.client.Watch(context.Background(), x.eventStreamPrefix, clientv3.WithPrefix(), clientv3.WithPrevKV())
	go func() {
		for v := range wch {
			for _, kv := range v.Events {
				_ = x.parseWatchEventStream(kv.Type, string(kv.Kv.Key), kv.Kv.Value)
			}
		}
	}()
	return nil
}

func (x *EventStream) parseWatchEventStream(op mvccpb.Event_EventType, key string, value []byte) (err error) {
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
	if op == mvccpb.DELETE {
		if b && actors != nil {
			actors.Delete(uint64(nodeId))
		}
		x.Logger().Warn("event stream key delete, success", "key", key)
		return nil
	} else {
		if actors == nil {
			actors = safemap.NewM[uint64, *ActorRef]()
			x.eventStreamMaps.Set(eventName, actors)
		}
		actorRef := newActorRefFromId(string(value))
		if actorRef == nil {
			return fmt.Errorf("invalid eventStream, id to actorRef err, key:%v", key)
		}
		actors.Set(uint64(nodeId), actorRef)
		x.Logger().Warn("event stream key add, success", "key", key)
	}
	return nil
}

func (x *EventStream) getEventNamePath(eventName string) string {
	str := strings.ReplaceAll(x.eventStreamPrefix+"/"+eventName+"/"+strconv.Itoa(int(x.nodeId)), "//", "/")
	return str
}

func (x *EventStream) registerEventStream(eventName string) {
	path := x.getEventNamePath(eventName)
	_, err := x.client.Put(context.Background(), path, x.Self().GetId(), clientv3.WithLease(x.leaseId))
	if err != nil {
		x.Logger().Error("register eventStream error", "path", path, "eventName", eventName, "err", err)
		return
	}
	//change local
	actors, _ := x.eventStreamMaps.Get(eventName)
	if actors == nil {
		actors = safemap.NewM[uint64, *ActorRef]()
		x.eventStreamMaps.Set(eventName, actors)
	}
	actors.Set(x.nodeId, x.Self())
}
func (x *EventStream) unregisterEventStream(eventName string) {
	path := x.getEventNamePath(eventName)
	_, err := x.client.Delete(context.Background(), path)
	if err != nil {
		x.Logger().Error("unregister eventStream error", "path", path, "eventName", eventName, "err", err)
		return
	}
	//change local
	actors, _ := x.eventStreamMaps.Get(eventName)
	actors.Delete(x.nodeId)
}
func (x *EventStream) getActorsByEventFromEventStream(event proto.Message) []*ActorRef {
	eventName := string(proto.MessageName(event))
	actors, b := x.eventStreamMaps.Get(eventName)
	if !b {
		return nil
	}
	var ret []*ActorRef
	actors.Range(func(nodeId uint64, ref *ActorRef) {
		ret = append(ret, ref)
	})
	return ret
}