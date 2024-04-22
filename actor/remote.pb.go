// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.30.0
// 	protoc        v3.5.1
// source: remote.proto

package actor

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type Envelope struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Header    map[string]string `protobuf:"bytes,1,rep,name=header,proto3" json:"header,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"` //for context k.v
	Sender    *ActorRef         `protobuf:"bytes,2,opt,name=sender,proto3" json:"sender,omitempty"`
	Target    *ActorRef         `protobuf:"bytes,3,opt,name=target,proto3" json:"target,omitempty"`
	RequestId uint64            `protobuf:"varint,4,opt,name=requestId,proto3" json:"requestId,omitempty"`
	MsgType   string            `protobuf:"bytes,5,opt,name=msgType,proto3" json:"msgType,omitempty"`
	Content   []byte            `protobuf:"bytes,6,opt,name=content,proto3" json:"content,omitempty"`
}

func (x *Envelope) Reset() {
	*x = Envelope{}
	if protoimpl.UnsafeEnabled {
		mi := &file_remote_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Envelope) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Envelope) ProtoMessage() {}

func (x *Envelope) ProtoReflect() protoreflect.Message {
	mi := &file_remote_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Envelope.ProtoReflect.Descriptor instead.
func (*Envelope) Descriptor() ([]byte, []int) {
	return file_remote_proto_rawDescGZIP(), []int{0}
}

func (x *Envelope) GetHeader() map[string]string {
	if x != nil {
		return x.Header
	}
	return nil
}

func (x *Envelope) GetSender() *ActorRef {
	if x != nil {
		return x.Sender
	}
	return nil
}

func (x *Envelope) GetTarget() *ActorRef {
	if x != nil {
		return x.Target
	}
	return nil
}

func (x *Envelope) GetRequestId() uint64 {
	if x != nil {
		return x.RequestId
	}
	return 0
}

func (x *Envelope) GetMsgType() string {
	if x != nil {
		return x.MsgType
	}
	return ""
}

func (x *Envelope) GetContent() []byte {
	if x != nil {
		return x.Content
	}
	return nil
}

var File_remote_proto protoreflect.FileDescriptor

var file_remote_proto_rawDesc = []byte{
	0x0a, 0x0c, 0x72, 0x65, 0x6d, 0x6f, 0x74, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x05,
	0x61, 0x63, 0x74, 0x6f, 0x72, 0x1a, 0x0b, 0x61, 0x63, 0x74, 0x6f, 0x72, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x22, 0x9e, 0x02, 0x0a, 0x08, 0x45, 0x6e, 0x76, 0x65, 0x6c, 0x6f, 0x70, 0x65, 0x12,
	0x33, 0x0a, 0x06, 0x68, 0x65, 0x61, 0x64, 0x65, 0x72, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32,
	0x1b, 0x2e, 0x61, 0x63, 0x74, 0x6f, 0x72, 0x2e, 0x45, 0x6e, 0x76, 0x65, 0x6c, 0x6f, 0x70, 0x65,
	0x2e, 0x48, 0x65, 0x61, 0x64, 0x65, 0x72, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x06, 0x68, 0x65,
	0x61, 0x64, 0x65, 0x72, 0x12, 0x27, 0x0a, 0x06, 0x73, 0x65, 0x6e, 0x64, 0x65, 0x72, 0x18, 0x02,
	0x20, 0x01, 0x28, 0x0b, 0x32, 0x0f, 0x2e, 0x61, 0x63, 0x74, 0x6f, 0x72, 0x2e, 0x41, 0x63, 0x74,
	0x6f, 0x72, 0x52, 0x65, 0x66, 0x52, 0x06, 0x73, 0x65, 0x6e, 0x64, 0x65, 0x72, 0x12, 0x27, 0x0a,
	0x06, 0x74, 0x61, 0x72, 0x67, 0x65, 0x74, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0f, 0x2e,
	0x61, 0x63, 0x74, 0x6f, 0x72, 0x2e, 0x41, 0x63, 0x74, 0x6f, 0x72, 0x52, 0x65, 0x66, 0x52, 0x06,
	0x74, 0x61, 0x72, 0x67, 0x65, 0x74, 0x12, 0x1c, 0x0a, 0x09, 0x72, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x49, 0x64, 0x18, 0x04, 0x20, 0x01, 0x28, 0x04, 0x52, 0x09, 0x72, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x49, 0x64, 0x12, 0x18, 0x0a, 0x07, 0x6d, 0x73, 0x67, 0x54, 0x79, 0x70, 0x65, 0x18,
	0x05, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6d, 0x73, 0x67, 0x54, 0x79, 0x70, 0x65, 0x12, 0x18,
	0x0a, 0x07, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x18, 0x06, 0x20, 0x01, 0x28, 0x0c, 0x52,
	0x07, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x1a, 0x39, 0x0a, 0x0b, 0x48, 0x65, 0x61, 0x64,
	0x65, 0x72, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x14, 0x0a, 0x05, 0x76, 0x61, 0x6c,
	0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x3a,
	0x02, 0x38, 0x01, 0x32, 0x3c, 0x0a, 0x08, 0x52, 0x65, 0x6d, 0x6f, 0x74, 0x69, 0x6e, 0x67, 0x12,
	0x30, 0x0a, 0x06, 0x4c, 0x69, 0x73, 0x74, 0x65, 0x6e, 0x12, 0x0f, 0x2e, 0x61, 0x63, 0x74, 0x6f,
	0x72, 0x2e, 0x45, 0x6e, 0x76, 0x65, 0x6c, 0x6f, 0x70, 0x65, 0x1a, 0x0f, 0x2e, 0x61, 0x63, 0x74,
	0x6f, 0x72, 0x2e, 0x45, 0x6e, 0x76, 0x65, 0x6c, 0x6f, 0x70, 0x65, 0x22, 0x00, 0x28, 0x01, 0x30,
	0x01, 0x42, 0x21, 0x5a, 0x1f, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f,
	0x63, 0x68, 0x65, 0x6e, 0x78, 0x79, 0x7a, 0x6c, 0x2f, 0x67, 0x72, 0x61, 0x69, 0x6e, 0x2f, 0x61,
	0x63, 0x74, 0x6f, 0x72, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_remote_proto_rawDescOnce sync.Once
	file_remote_proto_rawDescData = file_remote_proto_rawDesc
)

func file_remote_proto_rawDescGZIP() []byte {
	file_remote_proto_rawDescOnce.Do(func() {
		file_remote_proto_rawDescData = protoimpl.X.CompressGZIP(file_remote_proto_rawDescData)
	})
	return file_remote_proto_rawDescData
}

var file_remote_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_remote_proto_goTypes = []interface{}{
	(*Envelope)(nil), // 0: actor.Envelope
	nil,              // 1: actor.Envelope.HeaderEntry
	(*ActorRef)(nil), // 2: actor.ActorRef
}
var file_remote_proto_depIdxs = []int32{
	1, // 0: actor.Envelope.header:type_name -> actor.Envelope.HeaderEntry
	2, // 1: actor.Envelope.sender:type_name -> actor.ActorRef
	2, // 2: actor.Envelope.target:type_name -> actor.ActorRef
	0, // 3: actor.Remoting.Listen:input_type -> actor.Envelope
	0, // 4: actor.Remoting.Listen:output_type -> actor.Envelope
	4, // [4:5] is the sub-list for method output_type
	3, // [3:4] is the sub-list for method input_type
	3, // [3:3] is the sub-list for extension type_name
	3, // [3:3] is the sub-list for extension extendee
	0, // [0:3] is the sub-list for field type_name
}

func init() { file_remote_proto_init() }
func file_remote_proto_init() {
	if File_remote_proto != nil {
		return
	}
	file_actor_proto_init()
	if !protoimpl.UnsafeEnabled {
		file_remote_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Envelope); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_remote_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_remote_proto_goTypes,
		DependencyIndexes: file_remote_proto_depIdxs,
		MessageInfos:      file_remote_proto_msgTypes,
	}.Build()
	File_remote_proto = out.File
	file_remote_proto_rawDesc = nil
	file_remote_proto_goTypes = nil
	file_remote_proto_depIdxs = nil
}