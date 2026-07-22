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
	p := system.getRegistry().add(func() iProcess {
		return &processorReply[T]{
			registry: system.getRegistry(),
			_self:    self,
			result:   make(chan proto.Message, 1),
			timeout:  timeout,
		}
	}).(*processorReply[T])
	return p
}

func (x *processorReply[T]) self() ActorRef { return x._self }
func (x *processorReply[T]) opts() *tOpts   { return nil }
func (x *processorReply[T]) init()          {}

// poison wakes a caller blocked in Result() (e.g. during shutdown) by
// delivering the poison sentinel, which Result() maps to a "poisoned" error.
// Non-blocking: the buffered channel holds at most one value.
func (x *processorReply[T]) poison() {
	select {
	case x.result <- poison:
	default:
	}
}
func (x *processorReply[T]) send(ctx Context) {
	// non-blocking: the result channel is buffered for exactly one reply. A
	// duplicate reply (e.g. a retry) must not block the sender goroutine.
	select {
	case x.result <- ctx.Message():
	default:
	}
}

func (x *processorReply[T]) Result() (T, *message.ErrCode) {
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
		case *message.Poison:
			return null, message.WithErr("reply processor poisoned")
		case *message.ErrCode:
			return null, msg
		case error:
			return null, message.WithErr(msg.Error())
		default:
			return null, message.WithErr(fmt.Sprintf("msg type errr, need:%v, now:%v", null.ProtoReflect().Descriptor().FullName(), msg.ProtoReflect().Descriptor().FullName()))
		}
	case <-ctx.Done():
		return null, message.WithErr(errors.Join(ctx.Err(), fmt.Errorf("reply result timeout, id:%v", x.self())).Error())
	}
}
