package grain

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/chenxyzl/grain/al/safemap"
	"github.com/chenxyzl/grain/message"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/protobuf/proto"
)

var _ IActor = (*eventStream)(nil)

type eventStream struct {
	BaseActor
	client            *clientv3.Client
	leaseId           clientv3.LeaseID
	nodeId            uint64
	eventStreamPrefix string
	eventStreamMaps   *safemap.RWMap[string, *safemap.RWMap[uint64, ActorRef]] //eventName:eventStreamId:nodeId-actorId
	sub               map[string]map[string]bool                               // eventName:actorId:_
}

func newEventStream(nodeId uint64, client *clientv3.Client, leaseId clientv3.LeaseID, eventStreamPrefix string) IActor {
	return &eventStream{
		nodeId:            nodeId,
		client:            client,
		leaseId:           leaseId,
		eventStreamPrefix: eventStreamPrefix,
		eventStreamMaps:   safemap.NewRWMap[string, *safemap.RWMap[uint64, ActorRef]](),
		sub:               make(map[string]map[string]bool)}
}

func (x *eventStream) Started() {
	x.Logger().Info("EventStream started ...")
	//watcher eventStream
	err := x.watchEventStream()
	if err != nil {
		x.Logger().Error("watchEventStream err: ", "err", err)
		panic(err)
	}
}

func (x *eventStream) PreStop() {
	x.Logger().Info("EventStream stopped ...")
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

func (x *eventStream) parseWatchEventStream(op mvccpb.Event_EventType, key string, value []byte) (err error) {
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
	_, err := x.client.Put(context.Background(), path, x.Self().GetId(), clientv3.WithLease(x.leaseId))
	if err != nil {
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
	_, err := x.client.Delete(context.Background(), path)
	if err != nil {
		x.Logger().Error("unregister eventStream error", "path", path, "eventName", eventName, "err", err)
		return
	}
	//change local
	actors, _ := x.eventStreamMaps.Get(eventName)
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
