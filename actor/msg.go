package actor

import (
	"github.com/chenxyzl/grain/actor/internal"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var Msg = struct {
	//
	Poison proto.Message
	//inner
	poisonName   protoreflect.FullName
	streamClosed *internal.StreamClosed
	start        *internal.Start
	stop         *internal.Stop
}{
	//
	Poison: &Poison{},
	//inner
	poisonName:   (&Poison{}).ProtoReflect().Descriptor().FullName(),
	streamClosed: &internal.StreamClosed{},
	start:        &internal.Start{},
	stop:         &internal.Stop{},
}
