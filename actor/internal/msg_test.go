package internal

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestCustomProto(t *testing.T) {
	test := &BroadcastPublishProtoWrapper{&Initialize{}}
	v1 := test.ProtoReflect().Descriptor().FullName()
	fmt.Println(v1)
	v2 := proto.MessageName(test)
	fmt.Println(v2)
}
