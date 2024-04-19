package actor

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var Msg = struct {
	Poison     proto.Message
	PoisonName protoreflect.FullName
}{
	Poison:     &Poison{},
	PoisonName: (&Poison{}).ProtoReflect().Descriptor().FullName(),
}
