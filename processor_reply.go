package grain

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/chenxyzl/grain/message"
	"github.com/chenxyzl/grain/uuid"
	"google.golang.org/protobuf/proto"
)

var _ iProcess = (*processorReply[proto.Message])(nil)

type processorReply[T proto.Message] struct {
	registry iRegistry
	_self    ActorRef
	result   chan proto.Message
	timeout  time.Duration
}

func newProcessorReplay[T proto.Message](system ISystem, timeout time.Duration) *processorReply[T] {
	self := newDirectActorRef(defaultReplyKind, strconv.Itoa(int(uuid.Generate())), system.getAddr(), system)
	p := &processorReply[T]{
		registry: system.getRegistry(),
		_self:    self,
		result:   make(chan proto.Message, 1),
		timeout:  timeout,
	}
	p = p.registry.add(p).(*processorReply[T])
	return p
}

func (x *processorReply[T]) self() ActorRef { return x._self }

func (x *processorReply[T]) init()            {}
func (x *processorReply[T]) send(ctx Context) { x.result <- ctx.Message() }

func (x *processorReply[T]) Result() (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), x.timeout)
	defer func() {
		cancel()
		x.registry.remove(x._self)
	}()
	var null T
	select {
	case resp := <-x.result:
		switch msg := resp.(type) {
		case T:
			return msg, nil
		case *message.Error:
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
