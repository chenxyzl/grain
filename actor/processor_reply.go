package actor

import (
	"context"
	"errors"
	"fmt"
	"github.com/chenxyzl/grain/actor/internal"
	"github.com/chenxyzl/grain/uuid"
	"google.golang.org/protobuf/proto"
	"strconv"
	"time"
)

var _ iProcess = (*processorReply[proto.Message])(nil)

type processorReply[T proto.Message] struct {
	system  *System
	_self   *ActorRef
	result  chan proto.Message
	timeout time.Duration
}

func newProcessorReplay[T proto.Message](system *System, timeout time.Duration) *processorReply[T] {
	self := newActorRefWithKind(system.clusterProvider.addr(), defaultReplyKind, strconv.Itoa(int(uuid.Generate())))
	p := &processorReply[T]{
		system:  system,
		_self:   self,
		result:  make(chan proto.Message, 1),
		timeout: timeout,
	}
	p = system.registry.add(p).(*processorReply[T])
	return p
}

func (x *processorReply[T]) self() *ActorRef { return x._self }

func (x *processorReply[T]) init()            {}
func (x *processorReply[T]) send(ctx Context) { x.result <- ctx.Message() }

func (x *processorReply[T]) Result() (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), x.timeout)
	defer func() {
		cancel()
		x.system.registry.remove(x._self)
	}()
	var null T
	select {
	case resp := <-x.result:
		switch msg := resp.(type) {
		case T:
			return msg, nil
		case *internal.Error:
			return null, fmt.Errorf("grain error, code:%v, des:%s", msg.Code, msg.Des)
		case error:
			return null, msg
		default:
			return null, fmt.Errorf("result need %v, now: %v", null.ProtoReflect().Descriptor().FullName(), msg.ProtoReflect().Descriptor().FullName())
		}
	case <-ctx.Done():
		return null, errors.Join(ctx.Err(), fmt.Errorf("reply result timeout, id:%v", x.self()))
	}
}
