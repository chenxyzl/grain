package pb_test

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/known/anypb"
	"testing"
)

func TestPb(t *testing.T) {
	myMessage := &ActorRef{
		Address:    "xxx",
		Identifier: "John",
	}
	bytes, err := proto.Marshal(myMessage)
	if err != nil {
		t.Error(err)
	}
	testMsg := &TestMsg{Content: bytes}
	bytes1, err := proto.Marshal(testMsg)
	if err != nil {
		t.Error(err)
	}
	testMsg1 := &TestMsg{}
	err = proto.Unmarshal(bytes1, testMsg1)
	if err != nil {
		t.Error(err)
	}
	msg2 := &ActorRef{}
	err = proto.Unmarshal(testMsg1.Content, msg2)
	if err != nil {
		t.Error()
	}
	if msg2.Address != myMessage.Address {
		t.Error("unmarshaled message address mismatch")
	}
	if msg2.Identifier != myMessage.Identifier {
		t.Error("unmarshaled message identifier mismatch")
	}
}

func TestAny(t *testing.T) {
	myMessage := &ActorRef{
		Address:    "xxx",
		Identifier: "John",
	}
	any, err := anypb.New(myMessage)
	if err != nil {
		t.Error(err)
	}
	bytes, err := proto.Marshal(any)
	if err != nil {
		t.Error(err)
	}
	var unmarshaledAny anypb.Any
	err = proto.Unmarshal(bytes, &unmarshaledAny)
	if err != nil {
		t.Error(err)
	}
	if unmarshaledAny.MessageIs(&ActorRef{}) {
		// 解析 Any 对象中的 MyMessage 消息
		var msg2 ActorRef
		err := unmarshaledAny.UnmarshalTo(&msg2)
		if err != nil {
			t.Error(err)
		}
		if msg2.Address != myMessage.Address {
			t.Error("unmarshaled message address mismatch")
		}
		if msg2.Identifier != myMessage.Identifier {
			t.Error("unmarshaled message identifier mismatch")
		}
	}
}
func BenchmarkT(b *testing.B) {
	for i := 0; i < b.N; i++ {
		myMessage := &ActorRef{
			Address:    "xxx",
			Identifier: "John",
		}
		bytes, err := proto.Marshal(myMessage)
		if err != nil {
			b.Error(err)
		}
		testMsg := &TestMsg{Content: bytes}
		bytes1, err := proto.Marshal(testMsg)
		if err != nil {
			b.Error(err)
		}
		testMsg1 := &TestMsg{}
		err = proto.Unmarshal(bytes1, testMsg1)
		if err != nil {
			b.Error(err)
		}
		typ, err := protoregistry.GlobalTypes.FindMessageByName("actor.ActorRef")
		if err != nil {
			b.Error(err)
		}
		msg2 := typ.New().Interface().(*ActorRef)
		err = proto.Unmarshal(testMsg1.Content, msg2)
		if err != nil {
			b.Error()
		}
		if msg2.Address != myMessage.Address {
			b.Error("unmarshaled message address mismatch")
		}
		if msg2.Identifier != myMessage.Identifier {
			b.Error("unmarshaled message identifier mismatch")
		}
	}
}

func BenchmarkT1(b *testing.B) {
	for i := 0; i < b.N; i++ {
		myMessage := &ActorRef{
			Address:    "xxx",
			Identifier: "John",
		}
		bytes, err := proto.Marshal(myMessage)
		if err != nil {
			b.Error(err)
		}
		testMsg := &TestMsg{Content: bytes}
		bytes1, err := proto.Marshal(testMsg)
		if err != nil {
			b.Error(err)
		}
		testMsg1 := &TestMsg{}
		err = proto.Unmarshal(bytes1, testMsg1)
		if err != nil {
			b.Error(err)
		}
		msg2 := &ActorRef{}
		//msg2 := typ.New().Interface().(*ActorRef)
		err = proto.Unmarshal(testMsg1.Content, msg2)
		if err != nil {
			b.Error()
		}
		if msg2.Address != myMessage.Address {
			b.Error("unmarshaled message address mismatch")
		}
		if msg2.Identifier != myMessage.Identifier {
			b.Error("unmarshaled message identifier mismatch")
		}
	}
}
func BenchmarkT2(b *testing.B) {
	for i := 0; i < b.N; i++ {
		myMessage := &ActorRef{
			Address:    "xxx",
			Identifier: "John",
		}
		msg, err := anypb.New(myMessage)
		if err != nil {
			b.Error(err)
		}
		req := &Request{Content: msg}
		bytes, err := proto.Marshal(req)
		if err != nil {
			b.Error(err)
		}
		req1 := &Request{}
		err = proto.Unmarshal(bytes, req1)
		if err != nil {
			b.Error(err)
		}
		msg2, err := req1.Content.UnmarshalNew()
		if err != nil {
			b.Error(err)
		}
		msg3 := msg2.(*ActorRef)
		if myMessage.Address != msg3.Address {
			b.Error("unmarshaled message address mismatch")
		}
		if myMessage.Identifier != msg3.Identifier {
			b.Error("unmarshaled message identifier mismatch")
		}
	}
}
func BenchmarkT3(b *testing.B) {
	for i := 0; i < b.N; i++ {
		myMessage := &ActorRef{
			Address:    "xxx",
			Identifier: "John",
		}
		msg, err := anypb.New(myMessage)
		if err != nil {
			b.Error(err)
		}
		req := &Request{Content: msg}
		bytes, err := proto.Marshal(req)
		if err != nil {
			b.Error(err)
		}
		req1 := &Request{}
		err = proto.Unmarshal(bytes, req1)
		if err != nil {
			b.Error(err)
		}
		msg2 := &ActorRef{}
		err = req1.Content.UnmarshalTo(msg2)
		if err != nil {
			b.Error(err)
		}
		if myMessage.Address != msg2.Address {
			b.Error("unmarshaled message address mismatch")
		}
		if myMessage.Identifier != msg2.Identifier {
			b.Error("unmarshaled message identifier mismatch")
		}
	}
}
