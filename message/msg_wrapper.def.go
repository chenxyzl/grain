package message

import "google.golang.org/protobuf/proto"

var _ proto.Message = (*LocalProtoWrapper)(nil)

type LocalProtoWrapper struct{ proto.Message }
