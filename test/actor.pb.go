// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.30.0
// 	protoc        v3.5.1
// source: actor.proto

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

type ActorRef struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Address    string `protobuf:"bytes,1,opt,name=address,proto3" json:"address,omitempty"`       //server address
	Identifier string `protobuf:"bytes,2,opt,name=identifier,proto3" json:"identifier,omitempty"` //kind and key
}

func (x *ActorRef) Reset() {
	*x = ActorRef{}
	if protoimpl.UnsafeEnabled {
		mi := &file_actor_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ActorRef) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ActorRef) ProtoMessage() {}

func (x *ActorRef) ProtoReflect() protoreflect.Message {
	mi := &file_actor_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ActorRef.ProtoReflect.Descriptor instead.
func (*ActorRef) Descriptor() ([]byte, []int) {
	return file_actor_proto_rawDescGZIP(), []int{0}
}

func (x *ActorRef) GetAddress() string {
	if x != nil {
		return x.Address
	}
	return ""
}

func (x *ActorRef) GetIdentifier() string {
	if x != nil {
		return x.Identifier
	}
	return ""
}

type Identifier struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Kind string `protobuf:"bytes,1,opt,name=kind,proto3" json:"kind,omitempty"` //actor type
	Key  string `protobuf:"bytes,2,opt,name=key,proto3" json:"key,omitempty"`   //actor key
}

func (x *Identifier) Reset() {
	*x = Identifier{}
	if protoimpl.UnsafeEnabled {
		mi := &file_actor_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Identifier) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Identifier) ProtoMessage() {}

func (x *Identifier) ProtoReflect() protoreflect.Message {
	mi := &file_actor_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Identifier.ProtoReflect.Descriptor instead.
func (*Identifier) Descriptor() ([]byte, []int) {
	return file_actor_proto_rawDescGZIP(), []int{1}
}

func (x *Identifier) GetKind() string {
	if x != nil {
		return x.Kind
	}
	return ""
}

func (x *Identifier) GetKey() string {
	if x != nil {
		return x.Key
	}
	return ""
}

var File_actor_proto protoreflect.FileDescriptor

var file_actor_proto_rawDesc = []byte{
	0x0a, 0x0b, 0x61, 0x63, 0x74, 0x6f, 0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x05, 0x61,
	0x63, 0x74, 0x6f, 0x72, 0x22, 0x44, 0x0a, 0x08, 0x41, 0x63, 0x74, 0x6f, 0x72, 0x52, 0x65, 0x66,
	0x12, 0x18, 0x0a, 0x07, 0x61, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x07, 0x61, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x12, 0x1e, 0x0a, 0x0a, 0x69, 0x64,
	0x65, 0x6e, 0x74, 0x69, 0x66, 0x69, 0x65, 0x72, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a,
	0x69, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x66, 0x69, 0x65, 0x72, 0x22, 0x32, 0x0a, 0x0a, 0x49, 0x64,
	0x65, 0x6e, 0x74, 0x69, 0x66, 0x69, 0x65, 0x72, 0x12, 0x12, 0x0a, 0x04, 0x6b, 0x69, 0x6e, 0x64,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6b, 0x69, 0x6e, 0x64, 0x12, 0x10, 0x0a, 0x03,
	0x6b, 0x65, 0x79, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x42, 0x21,
	0x5a, 0x1f, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x63, 0x68, 0x65,
	0x6e, 0x78, 0x79, 0x7a, 0x6c, 0x2f, 0x67, 0x72, 0x61, 0x69, 0x6e, 0x2f, 0x61, 0x63, 0x74, 0x6f,
	0x72, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_actor_proto_rawDescOnce sync.Once
	file_actor_proto_rawDescData = file_actor_proto_rawDesc
)

func file_actor_proto_rawDescGZIP() []byte {
	file_actor_proto_rawDescOnce.Do(func() {
		file_actor_proto_rawDescData = protoimpl.X.CompressGZIP(file_actor_proto_rawDescData)
	})
	return file_actor_proto_rawDescData
}

var file_actor_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_actor_proto_goTypes = []interface{}{
	(*ActorRef)(nil),   // 0: actor.ActorRef
	(*Identifier)(nil), // 1: actor.Identifier
}
var file_actor_proto_depIdxs = []int32{
	0, // [0:0] is the sub-list for method output_type
	0, // [0:0] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_actor_proto_init() }
func file_actor_proto_init() {
	if File_actor_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_actor_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ActorRef); i {
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
		file_actor_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Identifier); i {
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
			RawDescriptor: file_actor_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_actor_proto_goTypes,
		DependencyIndexes: file_actor_proto_depIdxs,
		MessageInfos:      file_actor_proto_msgTypes,
	}.Build()
	File_actor_proto = out.File
	file_actor_proto_rawDesc = nil
	file_actor_proto_goTypes = nil
	file_actor_proto_depIdxs = nil
}
