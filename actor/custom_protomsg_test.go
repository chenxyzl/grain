package actor

import (
	"fmt"
	"github.com/chenxyzl/grain/actor/internal"
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestCustomProto(t *testing.T) {
	test := newPublishWrapper(&internal.Initialize{})
	v1 := test.ProtoReflect().Descriptor().FullName()
	fmt.Println(v1)
	v2 := proto.MessageName(test)
	fmt.Println(v2)
}
