package message

import (
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestCustomProto(t *testing.T) {
	test := &BroadcastPublishProtoWrapper{&Initialize{}}
	v1 := test.ProtoReflect().Descriptor().FullName()
	fmt.Println(v1)
	v2 := proto.MessageName(test)
	fmt.Println(v2)
}
