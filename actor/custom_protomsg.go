package actor

import (
	"google.golang.org/protobuf/proto"
)

var _ proto.Message = (*publishWrapper)(nil)

type publishWrapper struct {
	proto.Message
}

func newPublishWrapper(msg proto.Message) *publishWrapper {
	return &publishWrapper{msg}
}
