// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.30.0
// 	protoc        v3.5.1
// source: internal.proto

package internal

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

//Start --call Actor.Star
//Actor inner message
type Start struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *Start) Reset() {
	*x = Start{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Start) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Start) ProtoMessage() {}

func (x *Start) ProtoReflect() protoreflect.Message {
	mi := &file_internal_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Start.ProtoReflect.Descriptor instead.
func (*Start) Descriptor() ([]byte, []int) {
	return file_internal_proto_rawDescGZIP(), []int{0}
}

//Start --call Actor.Stop
//Actor inner message
type Stop struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *Stop) Reset() {
	*x = Stop{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Stop) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Stop) ProtoMessage() {}

func (x *Stop) ProtoReflect() protoreflect.Message {
	mi := &file_internal_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Stop.ProtoReflect.Descriptor instead.
func (*Stop) Descriptor() ([]byte, []int) {
	return file_internal_proto_rawDescGZIP(), []int{1}
}

//StreamClosed -- when StreamWrite stopped
//Actor inner message
type StreamClosed struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *StreamClosed) Reset() {
	*x = StreamClosed{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *StreamClosed) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*StreamClosed) ProtoMessage() {}

func (x *StreamClosed) ProtoReflect() protoreflect.Message {
	mi := &file_internal_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use StreamClosed.ProtoReflect.Descriptor instead.
func (*StreamClosed) Descriptor() ([]byte, []int) {
	return file_internal_proto_rawDescGZIP(), []int{2}
}

var File_internal_proto protoreflect.FileDescriptor

var file_internal_proto_rawDesc = []byte{
	0x0a, 0x0e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x12, 0x08, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x22, 0x07, 0x0a, 0x05, 0x53, 0x74,
	0x61, 0x72, 0x74, 0x22, 0x06, 0x0a, 0x04, 0x53, 0x74, 0x6f, 0x70, 0x22, 0x0e, 0x0a, 0x0c, 0x53,
	0x74, 0x72, 0x65, 0x61, 0x6d, 0x43, 0x6c, 0x6f, 0x73, 0x65, 0x64, 0x42, 0x2a, 0x5a, 0x28, 0x67,
	0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x63, 0x68, 0x65, 0x6e, 0x78, 0x79,
	0x7a, 0x6c, 0x2f, 0x67, 0x72, 0x61, 0x69, 0x6e, 0x2f, 0x61, 0x63, 0x74, 0x6f, 0x72, 0x2f, 0x69,
	0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_internal_proto_rawDescOnce sync.Once
	file_internal_proto_rawDescData = file_internal_proto_rawDesc
)

func file_internal_proto_rawDescGZIP() []byte {
	file_internal_proto_rawDescOnce.Do(func() {
		file_internal_proto_rawDescData = protoimpl.X.CompressGZIP(file_internal_proto_rawDescData)
	})
	return file_internal_proto_rawDescData
}

var file_internal_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_internal_proto_goTypes = []interface{}{
	(*Start)(nil),        // 0: internal.Start
	(*Stop)(nil),         // 1: internal.Stop
	(*StreamClosed)(nil), // 2: internal.StreamClosed
}
var file_internal_proto_depIdxs = []int32{
	0, // [0:0] is the sub-list for method output_type
	0, // [0:0] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_internal_proto_init() }
func file_internal_proto_init() {
	if File_internal_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_internal_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Start); i {
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
		file_internal_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Stop); i {
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
		file_internal_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*StreamClosed); i {
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
			RawDescriptor: file_internal_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_internal_proto_goTypes,
		DependencyIndexes: file_internal_proto_depIdxs,
		MessageInfos:      file_internal_proto_msgTypes,
	}.Build()
	File_internal_proto = out.File
	file_internal_proto_rawDesc = nil
	file_internal_proto_goTypes = nil
	file_internal_proto_depIdxs = nil
}
